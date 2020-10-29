# 謎のなにか予定地

ではあるんだけど、まず[Waypoint](https://www.waypointproject.io/)を試している。

```
$ brew tap hashicorp/tap
$ brew install hashicorp/tap/waypoint
$ waypoint install --platform=docker -accept-tos

$ git clone git@github.com:vvakame/se2gha.git
$ waypoint init
$ waypoint up
# waypoint build && waypoint deploy && waypoint release
$ waypoint ui
```

```
# got `Unable to list regions for project...` error
$ gcloud auth application-default login

# https://www.waypointproject.io/docs/troubleshooting
$ docker stop waypoint-server
$ docker rm waypoint-server
$ docker volume prune -f
```
