FROM --platform=$BUILDPLATFORM node:18 AS build
WORKDIR /app
COPY frontend/package*.json ./
RUN npm install
COPY frontend/ .
# Sentry release metadata threaded from CI. REACT_APP_SENTRY_RELEASE
# becomes available to the app bundle via CRA's build-time env
# substitution; the other two only matter for sentry-cli.
ARG REACT_APP_SENTRY_DSN=""
ARG REACT_APP_SENTRY_ENVIRONMENT="production"
ARG REACT_APP_SENTRY_RELEASE=""
ARG SENTRY_AUTH_TOKEN=""
ARG SENTRY_ORG=""
ARG SENTRY_PROJECT=""
# PostHog product analytics. KEY drives initialisation (empty → no-op
# in the running build); HOST defaults to PostHog Cloud US; ENVIRONMENT
# tags every event so prod / staging / dev stay separable in dashboards
# even when sharing one project key.
ARG REACT_APP_POSTHOG_KEY=""
ARG REACT_APP_POSTHOG_HOST="https://us.i.posthog.com"
ARG REACT_APP_POSTHOG_ENVIRONMENT="production"
ENV REACT_APP_SENTRY_DSN=$REACT_APP_SENTRY_DSN \
    REACT_APP_SENTRY_ENVIRONMENT=$REACT_APP_SENTRY_ENVIRONMENT \
    REACT_APP_SENTRY_RELEASE=$REACT_APP_SENTRY_RELEASE \
    REACT_APP_POSTHOG_KEY=$REACT_APP_POSTHOG_KEY \
    REACT_APP_POSTHOG_HOST=$REACT_APP_POSTHOG_HOST \
    REACT_APP_POSTHOG_ENVIRONMENT=$REACT_APP_POSTHOG_ENVIRONMENT
RUN npm run build

# Upload sourcemaps to Sentry when an auth token is present, then
# strip them from the build output so they never hit the browser.
# Skipping the upload cleanly when SENTRY_AUTH_TOKEN is empty keeps
# local `docker build` working without CI credentials.
RUN if [ -n "$SENTRY_AUTH_TOKEN" ] && [ -n "$REACT_APP_SENTRY_RELEASE" ]; then \
      npx --yes @sentry/cli@latest sourcemaps inject build/ && \
      npx --yes @sentry/cli@latest sourcemaps upload \
        --org "$SENTRY_ORG" \
        --project "$SENTRY_PROJECT" \
        --release "$REACT_APP_SENTRY_RELEASE" \
        build/ ; \
    else \
      echo "sentry: skipping sourcemap upload (SENTRY_AUTH_TOKEN or RELEASE unset)"; \
    fi && \
    find build/ -name "*.map" -delete

FROM --platform=linux/amd64 nginx:alpine
COPY --from=build /app/build /usr/share/nginx/html
COPY frontend/nginx.conf /etc/nginx/conf.d/default.conf