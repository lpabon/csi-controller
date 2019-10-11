FROM gcr.io/distroless/static:latest
LABEL maintainers="Kubernetes Authors"
LABEL description="CSI Controller"

COPY ./bin/csi-controller csi-controller
ENTRYPOINT ["/csi-controller"]
