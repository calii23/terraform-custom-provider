resource "custom_resource" "example" {
  type = "example"
  input = {
    "key" = "value"
  }
}
