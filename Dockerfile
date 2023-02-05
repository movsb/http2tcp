FROM scratch

COPY .build/http2tcp-$TARGETOS-TARGETARCH /http2tcp
CMD ["/http2tcp"]
