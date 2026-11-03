FROM golang:1.27-bookworm AS testes
WORKDIR /app
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
COPY testes ./testes
# Executa todos os testes com race; um pacote por vez reduz a disputa nas VMs.
RUN CGO_ENABLED=1 go test -race -p 1 -timeout 20m -v ./...
RUN go vet ./...

FROM testes AS compilacao
RUN mkdir /dados && chown 65532:65532 /dados
RUN CGO_ENABLED=0 go build -trimpath -o /bin/servidor ./cmd/servidor
RUN CGO_ENABLED=0 go build -trimpath -o /bin/cliente_motorista ./cmd/cliente_motorista
RUN CGO_ENABLED=0 go build -trimpath -o /bin/cliente_passageiro ./cmd/cliente_passageiro
RUN CGO_ENABLED=0 go build -trimpath -o /bin/carga ./cmd/carga

FROM scratch AS runtime
COPY --from=compilacao --chown=65532:65532 /dados /dados
ENV VAIJUNTO_BACKUP=/dados/estado.json
COPY --from=compilacao /bin/servidor /bin/servidor
COPY --from=compilacao /bin/cliente_motorista /bin/cliente_motorista
COPY --from=compilacao /bin/cliente_passageiro /bin/cliente_passageiro
COPY --from=compilacao /bin/carga /bin/carga
USER 65532:65532
EXPOSE 8080
EXPOSE 8081
ENTRYPOINT ["/bin/servidor"]
