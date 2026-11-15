# Creates EMPTY secret containers only. No aws_secretsmanager_secret_version lives here:
# values are put by hand with `aws secretsmanager put-secret-value` (docs/aws-setup.md), so
# they never touch Terraform code, tfvars or state. The one exception is "db", whose value
# modules/rds writes because RDS creates the password.
#
# Short names and shapes: docs/security.md#secrets (keep this map in sync with it).
locals {
  secrets = {
    "db"             = { required = "always", shape = "{\"username\",\"password\",\"host\",\"port\",\"dbname\"} (written by Terraform)" }
    "jwt"            = { required = "always", shape = "{\"userSigningKey\",\"adminSigningKey\"} base64 of 32 random bytes each" }
    "pii-keys"       = { required = "always", shape = "{\"current\":1,\"keys\":{\"1\":\"<base64 32 bytes>\"},\"hmacKey\":\"<base64 32 bytes>\"}" }
    "razorpay"       = { required = "when listing.payment_provider = razorpay", shape = "{\"keyId\",\"keySecret\",\"webhookSecret\"}" }
    "digilocker"     = { required = "when verification.provider = digilocker_direct", shape = "{\"clientId\",\"clientSecret\",\"redirectUri\",\"authorizeUrl\",\"tokenUrl\",\"apiBaseUrl\"}" }
    "kyc-aggregator" = { required = "when verification.provider = aggregator", shape = "{\"provider\",\"baseUrl\",\"apiKey\",\"apiSecret\",\"webhookSecret\"}" }
    "msg91"          = { required = "when auth.sms_provider = msg91", shape = "{\"authKey\",\"otpTemplateId\",\"guardianTemplateId\",\"senderId\"}" }
    "fcm"            = { required = "for push", shape = "{\"projectId\",\"clientEmail\",\"privateKey\"} from the Firebase service account" }
    "google-play"    = { required = "when selling on Android", shape = "{\"packageName\",\"clientEmail\",\"privateKey\",\"pubsubAudience\",\"pubsubServiceAccountEmail\"}" }
    "app-store"      = { required = "when selling on iOS", shape = "{\"bundleId\",\"issuerId\",\"keyId\",\"privateKey\",\"environment\":\"Sandbox|Production\",\"appAppleId\"}" }
  }
}

resource "aws_secretsmanager_secret" "this" {
  for_each = local.secrets

  name                    = "${var.prefix}/${each.key}"
  description             = "Athlix ${each.key} (required ${each.value.required}). Shape: ${each.value.shape}. Value set by hand, see docs/aws-setup.md."
  recovery_window_in_days = var.recovery_window_days

  tags = { SecretShortName = each.key }
}
