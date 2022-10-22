FROM scratch

COPY http2tcp /
ENTRYPOINT ["/http2tcp"]

