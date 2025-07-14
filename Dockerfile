FROM gcr.io/distroless/static-debian12:nonroot

ARG TARGETARCH

COPY bin/argocd-proxy-linux-${TARGETARCH} /bin/argocd-proxy

EXPOSE 5001
ENTRYPOINT ["/bin/argocd-proxy"]
