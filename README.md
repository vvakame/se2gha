# se2gha

```
$ git clone git@github.com:vvakame/se2gha.git
$ GCP_PROJECT_ID=vvakame-playground
$ GCP_REGION=us-central1
$ APPLICATION=se2gha
$ gcloud builds submit --tag "gcr.io/${GCP_PROJECT_ID}/${APPLICATION}:hogehoge"
$ gcloud run deploy "${APPLICATION}" \
    --project "${GCP_PROJECT_ID}" \
    --region "${GCP_REGION}" \
    --image "gcr.io/${GCP_PROJECT_ID}/${APPLICATION}:hogehoge" \
    --platform managed \
    --allow-unauthenticated \
    --no-traffic \
    --quiet
$ gcloud run services update-traffic "${APPLICATION}" \
    --to-latest \
    --region "${GCP_REGION}" \
    --platform managed \
    --quiet
```
