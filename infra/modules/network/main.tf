data "aws_availability_zones" "available" {
  state = "available"
}

locals {
  azs = slice(data.aws_availability_zones.available.names, 0, 2)

  # 10.x.0.0/24 and 10.x.1.0/24 public, 10.x.10.0/24 and 10.x.11.0/24 private.
  public_cidrs  = [for i in range(2) : cidrsubnet(var.vpc_cidr, 8, i)]
  private_cidrs = [for i in range(2) : cidrsubnet(var.vpc_cidr, 8, 10 + i)]

  # Interface endpoints are billed per AZ per hour, so dev can place them in one subnet.
  endpoint_subnet_ids = slice(aws_subnet.private[*].id, 0, var.interface_endpoint_subnet_count)
}

resource "aws_vpc" "this" {
  cidr_block           = var.vpc_cidr
  enable_dns_support   = true
  enable_dns_hostnames = true # required for private DNS on interface endpoints

  tags = { Name = var.name }
}

# Lock the default security group down: nothing should use it.
resource "aws_default_security_group" "this" {
  vpc_id = aws_vpc.this.id
  tags   = { Name = "${var.name}-default-unused" }
}

resource "aws_internet_gateway" "this" {
  vpc_id = aws_vpc.this.id
  tags   = { Name = var.name }
}

resource "aws_subnet" "public" {
  count = 2

  vpc_id                  = aws_vpc.this.id
  cidr_block              = local.public_cidrs[count.index]
  availability_zone       = local.azs[count.index]
  map_public_ip_on_launch = false

  tags = { Name = "${var.name}-public-${local.azs[count.index]}", Tier = "public" }
}

resource "aws_subnet" "private" {
  count = 2

  vpc_id            = aws_vpc.this.id
  cidr_block        = local.private_cidrs[count.index]
  availability_zone = local.azs[count.index]

  tags = { Name = "${var.name}-private-${local.azs[count.index]}", Tier = "private" }
}

resource "aws_route_table" "public" {
  vpc_id = aws_vpc.this.id
  tags   = { Name = "${var.name}-public" }
}

resource "aws_route" "public_internet" {
  route_table_id         = aws_route_table.public.id
  destination_cidr_block = "0.0.0.0/0"
  gateway_id             = aws_internet_gateway.this.id
}

resource "aws_route_table_association" "public" {
  count = 2

  subnet_id      = aws_subnet.public[count.index].id
  route_table_id = aws_route_table.public.id
}

# ONE NAT gateway for both AZs (cost choice): a NAT costs about USD 40/month per AZ before
# data. The trade-off is that an outage of the NAT's AZ cuts internet egress (DigiLocker,
# Razorpay, FCM, SMS, WebSocket management API) for Lambdas in the other AZ too. RDS, Redis,
# S3, SQS and Secrets Manager keep working (private / VPC endpoints). See README.md for how to
# move to one NAT per AZ.
resource "aws_eip" "nat" {
  domain = "vpc"
  tags   = { Name = "${var.name}-nat" }
}

resource "aws_nat_gateway" "this" {
  allocation_id = aws_eip.nat.id
  subnet_id     = aws_subnet.public[0].id
  tags          = { Name = var.name }

  depends_on = [aws_internet_gateway.this]
}

resource "aws_route_table" "private" {
  vpc_id = aws_vpc.this.id
  tags   = { Name = "${var.name}-private" }
}

resource "aws_route" "private_nat" {
  route_table_id         = aws_route_table.private.id
  destination_cidr_block = "0.0.0.0/0"
  nat_gateway_id         = aws_nat_gateway.this.id
}

resource "aws_route_table_association" "private" {
  count = 2

  subnet_id      = aws_subnet.private[count.index].id
  route_table_id = aws_route_table.private.id
}

# ---------------------------------------------------------------- security groups

# Attached to every Lambda. No ingress (Lambdas are invoked by AWS, never connected to).
resource "aws_security_group" "lambda" {
  name        = "${var.name}-lambda"
  description = "Athlix Lambdas: no ingress, egress to RDS, Redis, VPC endpoints and NAT"
  vpc_id      = aws_vpc.this.id
  tags        = { Name = "${var.name}-lambda" }
}

resource "aws_vpc_security_group_egress_rule" "lambda_all" {
  security_group_id = aws_security_group.lambda.id
  description       = "All egress (RDS, Redis, endpoints, partner APIs via NAT)"
  ip_protocol       = "-1"
  cidr_ipv4         = "0.0.0.0/0"
}

# Attached to the optional SSM port-forwarding instance (modules/ops_tunnel). No ingress:
# Session Manager connects outbound only.
resource "aws_security_group" "ops" {
  name        = "${var.name}-ops"
  description = "Optional SSM tunnel instance: no ingress, egress for SSM and DB or Redis"
  vpc_id      = aws_vpc.this.id
  tags        = { Name = "${var.name}-ops" }
}

resource "aws_vpc_security_group_egress_rule" "ops_all" {
  security_group_id = aws_security_group.ops.id
  description       = "All egress (SSM over 443, MySQL, Redis)"
  ip_protocol       = "-1"
  cidr_ipv4         = "0.0.0.0/0"
}

resource "aws_security_group" "endpoints" {
  count = var.enable_interface_endpoints ? 1 : 0

  name        = "${var.name}-endpoints"
  description = "Interface VPC endpoints: HTTPS from inside the VPC only"
  vpc_id      = aws_vpc.this.id
  tags        = { Name = "${var.name}-endpoints" }
}

resource "aws_vpc_security_group_ingress_rule" "endpoints_https" {
  count = var.enable_interface_endpoints ? 1 : 0

  security_group_id = aws_security_group.endpoints[0].id
  description       = "HTTPS from the VPC"
  ip_protocol       = "tcp"
  from_port         = 443
  to_port           = 443
  cidr_ipv4         = var.vpc_cidr
}

# ---------------------------------------------------------------- VPC endpoints

# S3 gateway endpoint: free, keeps media HeadObject/Delete traffic off the NAT.
resource "aws_vpc_endpoint" "s3" {
  vpc_id            = aws_vpc.this.id
  service_name      = "com.amazonaws.${var.region}.s3"
  vpc_endpoint_type = "Gateway"
  route_table_ids   = [aws_route_table.private.id]
  tags              = { Name = "${var.name}-s3" }
}

# Secrets Manager and SQS interface endpoints: every cold start reads secrets and every
# write publishes events, so these keep that traffic private and off NAT data charges.
resource "aws_vpc_endpoint" "interface" {
  for_each = var.enable_interface_endpoints ? toset(["secretsmanager", "sqs"]) : toset([])

  vpc_id              = aws_vpc.this.id
  service_name        = "com.amazonaws.${var.region}.${each.key}"
  vpc_endpoint_type   = "Interface"
  private_dns_enabled = true
  subnet_ids          = local.endpoint_subnet_ids
  security_group_ids  = [aws_security_group.endpoints[0].id]
  tags                = { Name = "${var.name}-${each.key}" }
}
