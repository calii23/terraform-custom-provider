provider "custom" {
  nodejs_resource {
    path     = "${path.module}/test.mjs"
    log_file = "${path.module}/log.txt"
  }

  nodejs_resource {
    path     = "${path.module}/api.mjs"
    log_file = "${path.module}/api.log"
    environment_variables = {
      API_TOKEN = "123456789"
    }
  }

  nodejs_data_source {
    path     = "${path.module}/data.mjs"
    log_file = "${path.module}/data.log"
  }
}
