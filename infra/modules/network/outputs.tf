output "vpc_id" {
  value = aws_vpc.this.id
}

output "vpc_cidr" {
  value = aws_vpc.this.cidr_block
}

output "private_subnet_ids" {
  value = aws_subnet.private[*].id
}

output "public_subnet_ids" {
  value = aws_subnet.public[*].id
}

output "lambda_security_group_id" {
  value = aws_security_group.lambda.id
}

output "ops_security_group_id" {
  value = aws_security_group.ops.id
}

output "nat_public_ip" {
  description = "Egress IP of every Lambda. Give this to partners that allowlist IPs (Razorpay, MSG91, KYC aggregator)."
  value       = aws_eip.nat.public_ip
}

output "nat_gateway_id" {
  value = aws_nat_gateway.this.id
}

output "endpoint_ids" {
  description = "Ids of the VPC endpoints, so dependants can wait until they exist."
  value       = concat([aws_vpc_endpoint.s3.id], [for e in aws_vpc_endpoint.interface : e.id])
}
