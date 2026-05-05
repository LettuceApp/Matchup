# Staging overlay

Standalone namespace (`matchup-staging`) in the same AKS cluster as
prod. Data plane is fully separated — its own Postgres volume, its
own Redis instance, its own Secret. The only thing staging shares
with prod is the cluster and the container registry (ACR).

## First-time setup

```bash
# 1. Set kubectl context to the AKS cluster.
az aks get-credentials --resource-group matchup-rg --name matchup-cluster

# 2. Create the Secret. Fill in real values (see secrets.yaml for
#    the full --from-literal list).
kubectl create secret generic matchup-secrets -n matchup-staging \
  --from-literal=DB_USER=… \
  --from-literal=DB_PASSWORD=… \
  --from-literal=API_SECRET=… \
  --from-literal=SENDGRID_API_KEY=… \
  --from-literal=SENDGRID_FROM=… \
  --from-literal=SENTRY_DSN=… \
  ...

# 3. Apply the overlay.
kubectl apply -k k8s/staging

# 4. Confirm.
kubectl get pods -n matchup-staging
curl -sS https://staging.matchup.app/health  # 200 OK once the cert issues
```

## What's shared vs separate

| Resource | Shared with prod | Separate |
|----------|------------------|----------|
| AKS cluster | ✔ | — |
| ACR registry | ✔ | — |
| cert-manager ClusterIssuer | ✔ | — |
| Postgres volume | — | Fresh PVC |
| Redis | — | Own Deployment + PVC |
| Secrets | — | Own `matchup-secrets` |
| Ingress host | — | `staging.matchup.app` |

## CI / redeploy

`.github/workflows/deploy-staging.yml` runs on every merge to `main` and:
1. Rebuilds both images with the new git SHA.
2. Applies the overlay (`kubectl apply -k k8s/staging`).
3. Waits for rollout.

Prod stays on the tagged-release path (see `deploy.yml`); staging
pushes happen automatically to keep it green for testing.
