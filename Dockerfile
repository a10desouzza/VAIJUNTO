FROM golang:1.27-bookworm AS testes
WORKDIR /app
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
COPY testes ./testes
RUN CGO_ENABLED=1 go test -race -v ./...
RUN go vet ./...

FROM testes AS compilacao
RUN CGO_ENABLED=0 go build -trimpath -o /bin/servidor ./cmd/servidor
RUN CGO_ENABLED=0 go build -trimpath -o /bin/cliente_motorista ./cmd/cliente_motorista
RUN CGO_ENABLED=0 go build -trimpath -o /bin/cliente_passageiro ./cmd/cliente_passageiro
RUN CGO_ENABLED=0 go build -trimpath -o /bin/carga ./cmd/carga

FROM scratch AS runtime
COPY --from=compilacao /bin/servidor /bin/servidor
COPY --from=compilacao /bin/cliente_motorista /bin/cliente_motorista
COPY --from=compilacao /bin/cliente_passageiro /bin/cliente_passageiro
COPY --from=compilacao /bin/carga /bin/carga
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/bin/servidor"]
