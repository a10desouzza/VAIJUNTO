# Protocolo TCP VaiJunto

Contrato implementado em `modelos.go` e `internal/servidor/roteador.go`. Não utiliza HTTP ou RPC.

## Mensagens e sessão

Uma requisição é um objeto JSON UTF-8, sem BOM, delimitado por LF (`\n`). CRLF também é aceito. O objeto deve ter menos de 1048576 bytes. Mensagens sem delimitador não são executadas. O servidor encerra conexões ociosas após 30 segundos, configurável por `-timeout`, e limita a 256 conexões simultâneas. Cada conexão processa as mensagens em ordem; conexões diferentes são concorrentes. O cliente espera até 5 segundos para conectar e 30 segundos por operação, e aceita respostas de até 16 MiB.

```json
{"acao":"CONSULTAR_RESERVAS","token":"TOKEN_DA_SESSAO","dados":{}}
```

```json
{"status":"SUCESSO","mensagem":"Operação concluída.","dados":[]}
```

```json
{"status":"ERRO","mensagem":"sessão inválida ou expirada; autentique-se"}
```

`CADASTRAR` e `AUTENTICAR` não exigem token. A autenticação devolve `token`, `expira_em` e `usuario`. O token vale duas horas, pode ser usado em novas conexões TCP e é revogado por `DESCONECTAR`. Fechar um socket não encerra a sessão. Senhas são armazenadas em RAM como derivação PBKDF2-SHA256 com salt aleatório; o transporte é TCP sem criptografia. Use credenciais de demonstração. Reiniciar o servidor apaga todo o estado.

## Operações

| Ação | Perfil | Dados | Retorno em `dados` |
|---|---|---|---|
| CADASTRAR | Público | nome, email, senha, perfil | Usuario |
| AUTENTICAR | Público | email, senha | Sessao |
| DESCONECTAR | Autenticado | objeto vazio | omitido |
| PUBLICAR_CARONA | MOTORISTA | chave, rota, data_hora, assentos, valor_km, trechos | Carona |
| CONSULTAR_CARONAS | MOTORISTA | objeto vazio | Lista das próprias caronas, inclusive canceladas, e passageiros ativos por trecho |
| CANCELAR_CARONA | MOTORISTA | id | Carona cancelada |
| BUSCAR_ITINERARIO | PASSAGEIRO | origem, destino, data, ordenar_por opcional, max_trechos opcional | ResultadoBusca |
| CONFIRMAR_RESERVA | PASSAGEIRO | chave, trechos_ids em ordem | Reserva |
| CONSULTAR_RESERVAS | PASSAGEIRO | objeto vazio | Lista das próprias reservas, inclusive canceladas |
| CANCELAR_RESERVA | PASSAGEIRO | id | Reserva cancelada |

`perfil`: MOTORISTA ou PASSAGEIRO. Senha: 8 a 128 bytes. Nome: 2 a 100 bytes. Cada conta possui um perfil. Email é normalizado para minúsculas.

`rota`: de 2 a 21 cidades distintas, até 100 bytes por nome. Espaços excedentes são normalizados e a busca ignora maiúsculas/minúsculas. `trechos` deve conter exatamente uma oferta por par consecutivo de cidades, com `distancia_km`, `tempo_viagem_min` e `tempo_parada_min`. O motorista informa `valor_km` na carona. O servidor calcula o preço por passageiro multiplicando valor/km pela distância e arredondando para centavos. Valor/km: de 0 a 1000000. Distância: positiva, até 100000 km. Ambos admitem até duas casas decimais; preço calculado por trecho limitado a 1000000. Viagem: de 1 a 10080 minutos. Parada: de 0 a 10080 minutos. `assentos`: de 1 a 100. Valor/km zero permite carona gratuita; parada omitida assume zero. IDs de caronas, trechos e reservas são gerados pelo servidor.

`data_hora`: partida inicial em RFC3339 com fuso. Os horários dos próximos trechos são calculados somando viagem e parada do trecho anterior. `data`: AAAA-MM-DD, comparada com a data da primeira partida no fuso da oferta. Conexões podem atravessar a meia-noite. A publicação exige partida futura. Novas reservas não são permitidas após a partida inicial de qualquer carona usada no itinerário, mesmo se o embarque seria em uma cidade intermediária.

Busca: caminhos sem repetir cidades e com vagas em todos os trechos. A próxima partida deve ser posterior ou igual à chegada anterior mais a parada mínima. Soma financeira em centavos; duração inclui esperas e exclui a parada após o destino final. `ordenar_por`: PRECO (padrão), TEMPO ou TRECHOS. Desempates: preço, duração, quantidade de trechos e IDs. `max_trechos`: padrão 12, máximo 20. Há limite de 100 resultados e 50000 arestas examinadas. `limitada: true` indica que a exploração foi limitada; a ordenação se aplica aos resultados encontrados, sem garantia de ótimo global nesse caso.

`chave`: identificador de 1 a 100 bytes escolhido pelo cliente para uma publicação ou confirmação. Repetir uma operação bem-sucedida com a mesma chave e conteúdo devolve o recurso existente sem duplicá-lo, inclusive se ele já tiver sido cancelado. Reutilizar a chave com dados diferentes é erro. Chaves são separadas por usuário e operação. Falhas não consomem a chave. Cancelamentos repetidos não devolvem assentos duas vezes.

Confirmar reserva aloca um assento numerado por trecho, sob um único RWMutex. Valida tudo antes de alterar qualquer trecho. A consulta não bloqueia vagas. Uma conexão interrompida antes da mensagem completa não cria reserva; se cair após a confirmação, consulte reservas ou repita a mesma chave. Uma reserva confirmada permanece até cancelamento. Cancelar uma carona cancela por inteiro as reservas que a utilizam e devolve os assentos dos demais trechos dessas reservas.

Campos desconhecidos, chaves JSON duplicadas, formatos incorretos e dados incompatíveis são rejeitados. Mensagens grandes demais encerram a conexão após tentativa de resposta de erro.

## Uso por terminal

Na raiz, inicie `go run ./cmd/servidor`. Os arquivos em `exemplos/` são payloads, sem envelope. O cliente monta o envelope.

```powershell
go run ./cmd/cliente_motorista -acao CADASTRAR -arquivo exemplos/cadastro_motorista.json
go run ./cmd/cliente_motorista -acao AUTENTICAR -arquivo exemplos/login_motorista.json
```

Copie o token retornado para `VAIJUNTO_TOKEN` ou passe `-token TOKEN`.

```powershell
$env:VAIJUNTO_TOKEN = 'TOKEN_DO_MOTORISTA'
go run ./cmd/cliente_motorista -acao PUBLICAR_CARONA -arquivo exemplos/publicar.json
go run ./cmd/cliente_motorista -acao CONSULTAR_CARONAS
```

```powershell
go run ./cmd/cliente_passageiro -acao CADASTRAR -arquivo exemplos/cadastro_passageiro.json
go run ./cmd/cliente_passageiro -acao AUTENTICAR -arquivo exemplos/login_passageiro.json
$env:VAIJUNTO_TOKEN = 'TOKEN_DO_PASSAGEIRO'
go run ./cmd/cliente_passageiro -acao BUSCAR_ITINERARIO -arquivo exemplos/buscar.json
go run ./cmd/cliente_passageiro -acao CONFIRMAR_RESERVA -arquivo exemplos/reservar.json
go run ./cmd/cliente_passageiro -acao CONSULTAR_RESERVAS
go run ./cmd/cliente_passageiro -acao CANCELAR_RESERVA -arquivo exemplos/cancelar_reserva.json
```

Os IDs de exemplo valem para a primeira publicação e reserva de um servidor vazio. Ajuste os arquivos pelos IDs realmente retornados. Também há `-dados` para JSON diretamente e `-dados -` para stdin.

## Execução distribuída

```powershell
docker compose build
docker compose up -d servidor
docker compose run --rm motorista -acao CADASTRAR -dados -
docker compose run --rm carga -clientes 100 -vagas 5
```

A construção da imagem executa testes com `-race` e `go vet` no estágio `testes`. O servidor publica a porta TCP 8080 do host. Na máquina cliente do laboratório, construa a imagem e configure o IP LAN real da máquina servidor, sem iniciar outro servidor:

```powershell
$env:VAIJUNTO_SERVIDOR = '192.168.1.10:8080'
docker compose run --rm passageiro -acao AUTENTICAR -dados -
docker compose run --rm carga -clientes 100 -vagas 5
```

Substitua o IP pelo endereço real do laboratório e permita TCP 8080 no firewall do servidor. O nome `servidor` só resolve na rede Compose local; em hosts distintos usa-se o IP LAN e a porta publicada. Não use localhost para alcançar outra máquina.

Carga nativa: `go run ./cmd/carga -servidor 127.0.0.1:8080 -clientes 100 -vagas 5`. Cria contas e duas caronas de motoristas distintos para o teste; ao terminar, cancela as caronas. Mede apenas a fase concorrente de confirmação: duração, média, p95, vazão, confirmações, recusas e erros de transporte. Os cadastros e o histórico de teste permanecem em RAM. O processo retorna código diferente de zero se falhar a integridade ou o transporte.

## Regras de cancelamento e avisos

`CANCELAR_TRECHO` recebe `dados: {"id":"ID_DO_TRECHO"}` e exige token do motorista proprietário. Cancela apenas a oferta daquele trecho; os demais trechos permanecem ativos. A carona fica `PARCIALMENTE_CANCELADA` ou `CANCELADA` se todos os seus trechos foram cancelados. Toda reserva ativa que inclui o trecho cancelado é cancelada por inteiro, devolvendo as vagas em todos os seus trechos uma única vez.

Cancelamentos do motorista, de trecho ou carona inteira, são recusados a partir da partida inicial da carona. Também são recusados quando qualquer reserva afetada já iniciou o primeiro trecho do seu itinerário, mesmo que esse primeiro trecho pertença a outro motorista. A validação ocorre sob a mesma trava de escrita, antes de mudar vagas, reservas ou notificações. Um passageiro também não pode cancelar a própria reserva depois da primeira partida do itinerário.

`CONSULTAR_NOTIFICACOES` recebe objeto vazio e retorna somente os avisos do usuário autenticado. Cada aviso contém `id`, `reserva_id`, `mensagem`, `criada_em` e `lida`. `LER_NOTIFICACAO` recebe `dados: {"id":"ID_DO_AVISO"}` e marca o aviso do próprio usuário como lido. Consultar não remove avisos; marcar leitura novamente não causa efeitos adicionais.

O servidor cria o aviso de cancelamento na mesma seção crítica que altera a reserva. O cliente passageiro consulta avisos ao entrar e retornar ao menu; a opção 5, Verificar notificações, permite ver também o histórico. Não há envio espontâneo enquanto o usuário está parado preenchendo um campo. Os avisos permanecem em RAM durante desconexões do cliente, mas se perdem ao encerrar o servidor.

No menu do motorista, a opção 5 cancela um trecho. A opção 2 mostra a quantidade atual de vagas e os passageiros por trecho. A interface não oferece escolha nem exibe número de assento: cada reserva ocupa uma vaga por trecho. Os números usados internamente servem ao controle e aos testes de ocupação.

Publicações com a mesma rota e chaves distintas criam caronas e trechos com IDs distintos. Repetir a mesma chave e os mesmos dados continua sendo uma repetição idempotente da publicação anterior.

Os menus são abertos sem `-acao`: `go run ./cmd/cliente_motorista` e `go run ./cmd/cliente_passageiro`. A busca continua consultiva; a trava protege a confirmação e é liberada antes do envio da resposta TCP.