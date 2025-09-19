from golang:alpine as builder

COPY main.go /app/main.go

RUN go build -o /app/l4proxy /app/main.go

FROM scratch

COPY --from=builder /app/l4proxy /l4proxy

CMD ["/l4proxy"]
ENTRYPOINT ["/l4proxy"]
