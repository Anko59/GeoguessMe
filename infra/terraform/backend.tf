terraform {
  # Supply R2's S3 endpoint and bucket-scoped credentials through
  # backend.hcl/environment variables. use_lockfile provides state locking.
  backend "s3" {}
}
