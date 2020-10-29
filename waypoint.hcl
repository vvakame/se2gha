project = "se2gha"

# I'm using GCP project named "vvakame-playground".

app "se2gha" {
  build {
    use "pack" {
      builder = "gcr.io/buildpacks/builder:v1"
    }

    registry {
      use "docker" {
        image = "gcr.io/vvakame-playground/se2gha"
        tag   = gitrefpretty()
      }
    }
  }

  deploy {
    use "google-cloud-run" {
      project  = "vvakame-playground"
      location = "us-central1"

      port = 8080

      static_environment = {
        "NAME" = "Hello, World"
      }

      unauthenticated = true

      capacity {
        memory                     = 128
        cpu_count                  = 1
        max_requests_per_container = 10
        request_timeout            = 30
      }

      auto_scaling {
        max = 1
      }
    }
  }
  release {
    use "google-cloud-run" {}
  }
}
