# se2gha

```
$ git clone git@github.com:vvakame/se2gha.git
$ GCP_PROJECT_ID=vvakame-playground
$ GCP_REGION=us-central1
$ SERVICE_NAME=se2gha
$ gcloud builds submit --tag "gcr.io/${GCP_PROJECT_ID}/${SERVICE_NAME}"
$ gcloud run deploy "${SERVICE_NAME}" \
    --project "${GCP_PROJECT_ID}" \
    --region "${GCP_REGION}" \
    --image "gcr.io/${GCP_PROJECT_ID}/${APPLICATION}" \
    --platform managed \
    --allow-unauthenticated \
    --no-traffic \
    --quiet
$ gcloud run services update-traffic "${SERVICE_NAME}" \
    --to-latest \
    --region "${GCP_REGION}" \
    --platform managed \
    --quiet
```
