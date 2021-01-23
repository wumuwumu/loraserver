FROM alpine:latest

WORKDIR /root/
RUN apk --no-cache add ca-certificates tzdata
COPY ./build/linux/loraserver .
ENTRYPOINT ["./loraserver"]
