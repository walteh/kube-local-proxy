from golang:alpine as builder

COPY main.go /app/main.go

RUN go build -o /app/proxymux /app/main.go

FROM scratch

COPY --from=builder /app/proxymux /proxymux

CMD ["/proxymux"]
