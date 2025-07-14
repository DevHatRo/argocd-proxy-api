FROM gcr.io/distroless/static-debian12:nonroot

ARG TARGETARCH

COPY bin/argocd-proxy-api-linux-${TARGETARCH} /bin/argocd-proxy-api

EXPOSE 5001
ENTRYPOINT ["/bin/argocd-proxy-api"]
