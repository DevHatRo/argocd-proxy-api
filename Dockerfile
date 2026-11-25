FROM gcr.io/distroless/static-debian13:nonroot

ARG TARGETARCH

COPY --chmod=755 bin/argocd-proxy-api-linux-${TARGETARCH} /app/argocd-proxy-api

EXPOSE 5001
ENTRYPOINT ["/app/argocd-proxy-api"]
