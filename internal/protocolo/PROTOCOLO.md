# Protocolo TCP VaiJunto

Contrato implementado em `modelos.go` e `internal/servidor/roteador.go`. Não utiliza HTTP ou RPC.

## Mensagens e sessão

Uma requisição é um objeto JSON UTF-8, sem BOM, delimitado por LF (`\n`). CRLF também é aceito. O objeto deve ter menos de 1048576 bytes. Mensagens sem delimitador não são executadas. O servidor encerra conexões ociosas após 30 segundos, configurável por `-timeout`, e limita a 256 conexões simultâneas. Cada conexão processa as mensagens em ordem; conexões diferentes são concorrentes. O cliente espera até 5 segundos para conectar e 30 segundos por operação, e aceita respostas de até 16 MiB.

```json
{"acao":"CONSULTAR_RESERVAS","sessao":"ID_DA_SESSAO","dados":{}}
```

```json
{"status":"SUCESSO","mensagem":"Operação concluída.","dados":[]}
```

```json
{"status":"ERRO","mensagem":"sessão inválida ou expirada; autentique-se"}
```

`CADASTRAR` cria a conta e já inicia o acesso. `AUTENTICAR` continua disponível para contas existentes. As duas operações devolvem `id`, `expira_em` e `usuario`. O identificador vale 15 minutos a partir do login, sem renovação por atividade, é administrado internamente pelo cliente e é revogado por `DESCONECTAR`. Fechar um socket não encerra a sessão. Senhas são armazenadas em RAM como derivação PBKDF2-SHA256 com salt aleatório; o transporte é TCP sem criptografia. Use credenciais de demonstração. Reiniciar o servidor apaga todo o estado.

## Operações

| Ação | Perfil | Dados | Retorno em `dados` |
|---|---|---|---|
| CADASTRAR | Público | nome, email, senha, perfil | Sessao |
| AUTENTICAR | Público | email, senha | Sessao |
| DESCONECTAR | Autenticado | objeto vazio | omitido |
| PUBLICAR_CARONA | MOTORISTA | chave, rota, data_hora, assentos, trechos | Carona |
| CONSULTAR_CARONAS | MOTORISTA | objeto vazio | Lista das próprias caronas, inclusive canceladas, e passageiros ativos por trecho |
| CANCELAR_CARONA | MOTORISTA | id | Carona cancelada |
| BUSCAR_ITINERARIO | PASSAGEIRO | origem, destino, data, max_trechos opcional | ResultadoBusca |
| CONFIRMAR_RESERVA | PASSAGEIRO | chave, trechos_ids em ordem | Reserva |
| CONSULTAR_RESERVAS | PASSAGEIRO | objeto vazio | Lista das próprias reservas, inclusive canceladas |
| CANCELAR_RESERVA | PASSAGEIRO | id | Reserva cancelada |
| CANCELAR_TRECHO | MOTORISTA | id do trecho | Carona atualizada |
| CONSULTAR_NOTIFICACOES | Autenticado | objeto vazio | Lista de Notificacao do próprio usuário |
| LER_NOTIFICACAO | Autenticado | id do aviso | omitido |

`perfil`: MOTORISTA ou PASSAGEIRO. Senha: no mínimo 8 caracteres Unicode e no máximo 128 bytes UTF-8, sem caracteres de controle. Nome: ao menos duas letras e até 100 bytes, admitindo espaços, marcas de acentuação, hífen, apóstrofo e ponto. Cada conta possui um perfil. Email é normalizado para minúsculas, limitado a 254 bytes e validado sintaticamente, sem verificar a existência da caixa postal.

`rota`: de 2 a 21 cidades distintas, até 100 bytes por nome. Espaços excedentes são normalizados e a busca ignora maiúsculas/minúsculas. `trechos` contém uma oferta por par consecutivo de cidades, com `preco`, `distancia_km`, `tempo_viagem_min` e `tempo_parada_min`. O motorista escolhe o preço por passageiro de cada trecho, entre 0 e 1000000 reais, com até duas casas decimais. Zero permite carona gratuita. A distância é informativa: deve ser positiva, até 100000 km, com até duas casas decimais. Viagem: de 1 a 10080 minutos. Parada: de 0 a 10080 minutos. `assentos`: de 1 a 100. IDs de caronas, trechos e reservas são gerados pelo servidor.

`data_hora`: partida inicial em RFC3339 com fuso. Os horários dos próximos trechos são calculados somando viagem e parada do trecho anterior. `data`: AAAA-MM-DD, comparada com a data da primeira partida no fuso da oferta. Conexões podem atravessar a meia-noite. A publicação exige partida futura. Novas reservas são permitidas somente antes da partida de cada trecho solicitado. Trechos futuros continuam disponíveis durante a viagem anterior e a parada intermediária, mesmo com a carona já iniciada. No instante exato da partida do trecho, novas reservas são recusadas.

Busca: caminhos sem repetir cidades e com vagas em todos os trechos. A próxima partida deve ser posterior ou igual à chegada anterior mais a parada mínima. Soma financeira em centavos; duração inclui esperas e exclui a parada após o destino final. Ordenação única pelo horário de partida do primeiro trecho, em ordem crescente e comparando instantes com seus fusos. Desempates: preço, duração, quantidade de trechos e IDs. `max_trechos`: padrão 12, máximo 20. Há limite de 100 resultados e 50000 arestas examinadas. `limitada: true` indica que a exploração foi limitada; a ordenação se aplica aos resultados encontrados, sem garantia de ótimo global nesse caso.

`chave`: identificador de 1 a 100 bytes escolhido pelo cliente para uma publicação ou confirmação. Repetir uma operação bem-sucedida com a mesma chave e conteúdo devolve o recurso existente sem duplicá-lo, inclusive se ele já tiver sido cancelado. Reutilizar a chave com dados diferentes é erro. Chaves são separadas por usuário e operação. Falhas não consomem a chave. Cancelamentos repetidos não devolvem assentos duas vezes.

Confirmar reserva aloca um assento numerado por trecho, sob um único RWMutex. Valida tudo antes de alterar qualquer trecho. A consulta não bloqueia vagas. Uma conexão interrompida antes da mensagem completa não cria reserva; se cair após a confirmação, consulte reservas ou repita a mesma chave. Uma reserva confirmada permanece até cancelamento. Cancelar uma carona cancela por inteiro as reservas que a utilizam e devolve os assentos dos demais trechos dessas reservas.

Campos desconhecidos, chaves JSON duplicadas, formatos incorretos e dados incompatíveis são rejeitados. Mensagens grandes demais encerram a conexão após tentativa de resposta de erro.

## Formatos e campos das respostas

`acao`, `sessao` e todos os IDs são strings. `dados` da requisição deve ser um objeto, inclusive quando vazio (`{}`). `sessao` pode ser omitida nas operações públicas; nas demais, precisa identificar uma sessão válida. A resposta sempre contém `status` (`SUCESSO` ou `ERRO`) e `mensagem` (string). `dados` é omitido em erros e nos sucessos sem retorno. Não há código numérico de erro: o cliente deve verificar `status`, exibindo `mensagem`, sem depender de um texto de erro fixo.

Datas de calendário usam `AAAA-MM-DD`; instantes usam RFC3339 com fuso (`2099-10-01T08:00:00-03:00` ou `2099-10-01T11:00:00Z`). Dinheiro é um número JSON em reais por passageiro, não uma string com `R$`; distâncias são números em quilômetros. Minutos, capacidades e números de assento são inteiros; `limitada` e `lida` são booleanos. Listas sem elementos são `[]`. A ordem dos campos JSON não é significativa.

Os campos abaixo estão presentes nas respostas, salvo os dois explicitamente opcionais de `Reserva`. Os nomes dos tipos são apenas referências desta documentação: não são campos adicionais no JSON.

| Tipo | Campos e significado |
| :--- | :--- |
| Usuario | `nome`, `email`, `perfil`: strings; nunca contém senha, salt ou hash |
| Sessao | `id`: identificador aleatório interno; `expira_em`: instante de expiração; `usuario`: Usuario |
| Trecho | `id`, `carona_id`: identificadores; `origem`, `destino`: cidades; `motorista_email`: proprietário; `status`: `ATIVA` ou `CANCELADA`; `data_hora`: partida deste trecho; `distancia_km`: distância; `preco`: preço escolhido pelo motorista; `tempo_viagem_min`: duração da viagem; `tempo_parada_min`: parada após chegada; `capacidade`: total de vagas; `assentos_livres`: vagas disponíveis |
| PassageiroConfirmado | `reserva_id`: reserva ativa; `passageiro`: Usuario; `assento`: número atribuído neste trecho |
| TrechoConsultado | `trecho`: Trecho; `passageiros`: lista de PassageiroConfirmado, ordenada pelo assento |
| Carona | `id`, `motorista_email`: strings; `rota`: lista ordenada de cidades; `data_hora`: partida inicial; `status`: `ATIVA`, `PARCIALMENTE_CANCELADA` ou `CANCELADA`; `trechos`: lista ordenada de TrechoConsultado |
| Itinerario | `trechos`: lista ordenada de Trecho; `preco_total`: soma dos preços; `duracao_total_min`: minutos da primeira partida à última chegada, incluindo conexões |
| ResultadoBusca | `itinerarios`: lista de Itinerario; `limitada`: indica exploração limitada; `max_trechos`: limite efetivo usado |
| AssentoReservado | `trecho`: cópia do Trecho na confirmação; `numero`: assento atribuído automaticamente, de 1 até a capacidade |
| Reserva | `id`, `passageiro_email`: strings; `status`: `ATIVA` ou `CANCELADA`; `criada_em`: instante de criação; `assentos`: lista ordenada de AssentoReservado; `preco_total`: soma; `cancelada_em` e `motivo`: strings opcionais, presentes após cancelamento |
| Notificacao | `id`, `reserva_id`, `mensagem`: strings; `criada_em`: instante da criação; `lida`: booleano |

`CONSULTAR_CARONAS` e `CONSULTAR_RESERVAS` devolvem listas ordenadas por ID. As caronas mostram ocupação atual. Os trechos dentro de uma reserva são cópias históricas da confirmação, não consultas às vagas atuais; o estado do cancelamento está na própria reserva. A busca também é uma fotografia: vagas podem mudar antes da confirmação.

### Campos de entrada e valores padrão

Envie todos os campos listados na tabela de operações, exceto os opcionais descritos aqui. Campos escalares omitidos são decodificados com o valor zero de Go e passam pelas mesmas validações; isso não torna válido omitir nome, cidades, duração de viagem, chave ou outros valores cuja regra rejeita zero/vazio. `dados` não pode ser omitido nem ser `null`.

- `max_trechos` omitido ou zero usa 12; valores explícitos válidos são de 1 a 20.
- `preco` omitido em uma oferta de trecho assume zero (trecho gratuito); envie-o explicitamente para evitar ambiguidade.
- `tempo_parada_min` omitido assume zero. Só faz sentido nos trechos anteriores ao último da **carona publicada**. No último trecho, o servidor normaliza um valor válido para zero, antes de calcular a assinatura de idempotência. Valores negativos ou acima de 10080 continuam inválidos. O menu não solicita esse campo no destino final.
- `trechos_ids` é uma lista de 1 a 20 IDs, na ordem da viagem, sem repetições; os trechos precisam formar um caminho válido.

Uma carona com A → B → C possui dois trechos. A parada do primeiro ocorre em B; a do segundo é sempre zero. Uma outra carona que saia de C pode conectar a partir da chegada em C, sem espera artificial. Caso o passageiro saia de uma carona em uma cidade intermediária, a regra de conexão ainda respeita a parada mínima daquele trecho.

## Exemplos completos de mensagens

Os blocos são objetos JSON completos; na rede cada mensagem deve ocupar uma linha, seguida de LF. IDs e sessões são ilustrativos: substitua-os pelos retornados pelo servidor. Horários de criação/expiração também são ilustrativos. Os exemplos de consulta vazia e cancelamento são alternativas, não uma sequência obrigatória.

### Cadastro com acesso automático e autenticação

Requisição de cadastro:

```json
{"acao":"CADASTRAR","dados":{"nome":"Ana Souza","email":"ana@exemplo.com","senha":"senha1234","perfil":"MOTORISTA"}}
```

Resposta (a sessão já permite publicar):

```json
{"status":"SUCESSO","mensagem":"Operação concluída.","dados":{"id":"SESSAO_ANA","expira_em":"2099-10-01T12:00:00Z","usuario":{"nome":"Ana Souza","email":"ana@exemplo.com","perfil":"MOTORISTA"}}}
```

Para uma conta existente:

```json
{"acao":"AUTENTICAR","dados":{"email":"ana@exemplo.com","senha":"senha1234"}}
```

```json
{"status":"SUCESSO","mensagem":"Operação concluída.","dados":{"id":"NOVA_SESSAO_ANA","expira_em":"2099-10-01T12:10:00Z","usuario":{"nome":"Ana Souza","email":"ana@exemplo.com","perfil":"MOTORISTA"}}}
```

O passageiro se cadastra da mesma forma, com `perfil: "PASSAGEIRO"`. Nos exemplos seguintes, `SESSAO_PASSAGEIRO` pertence à conta `bia@exemplo.com` previamente cadastrada.

### Publicar uma carona

```json
{"acao":"PUBLICAR_CARONA","sessao":"SESSAO_ANA","dados":{"chave":"publicacao-1","rota":["Salvador","Feira de Santana"],"data_hora":"2099-10-01T08:00:00-03:00","assentos":2,"trechos":[{"preco":50,"distancia_km":100,"tempo_viagem_min":120,"tempo_parada_min":0}]}}
```

```json
{"status":"SUCESSO","mensagem":"Operação concluída.","dados":{"id":"C000001","motorista_email":"ana@exemplo.com","rota":["Salvador","Feira de Santana"],"data_hora":"2099-10-01T08:00:00-03:00","status":"ATIVA","trechos":[{"trecho":{"status":"ATIVA","distancia_km":100,"id":"T000002","carona_id":"C000001","origem":"Salvador","destino":"Feira de Santana","data_hora":"2099-10-01T08:00:00-03:00","assentos_livres":2,"capacidade":2,"preco":50,"tempo_viagem_min":120,"tempo_parada_min":0,"motorista_email":"ana@exemplo.com"},"passageiros":[]}]}}
```

### Buscar itinerário

```json
{"acao":"BUSCAR_ITINERARIO","sessao":"SESSAO_PASSAGEIRO","dados":{"origem":"Salvador","destino":"Feira de Santana","data":"2099-10-01","max_trechos":12}}
```

```json
{"status":"SUCESSO","mensagem":"Operação concluída.","dados":{"itinerarios":[{"trechos":[{"status":"ATIVA","distancia_km":100,"id":"T000002","carona_id":"C000001","origem":"Salvador","destino":"Feira de Santana","data_hora":"2099-10-01T08:00:00-03:00","assentos_livres":2,"capacidade":2,"preco":50,"tempo_viagem_min":120,"tempo_parada_min":0,"motorista_email":"ana@exemplo.com"}],"preco_total":50,"duracao_total_min":120}],"limitada":false,"max_trechos":12}}
```

Sem caminhos encontrados, o sucesso tem `dados: {"itinerarios":[],"limitada":false,"max_trechos":12}` (ou `limitada: true` se a exploração foi limitada). Não encontrar caminho não é erro de protocolo.

### Confirmar reserva

```json
{"acao":"CONFIRMAR_RESERVA","sessao":"SESSAO_PASSAGEIRO","dados":{"chave":"reserva-1","trechos_ids":["T000002"]}}
```

```json
{"status":"SUCESSO","mensagem":"Operação concluída.","dados":{"id":"R000003","passageiro_email":"bia@exemplo.com","status":"ATIVA","criada_em":"2099-10-01T10:15:00Z","assentos":[{"trecho":{"status":"ATIVA","distancia_km":100,"id":"T000002","carona_id":"C000001","origem":"Salvador","destino":"Feira de Santana","data_hora":"2099-10-01T08:00:00-03:00","assentos_livres":1,"capacidade":2,"preco":50,"tempo_viagem_min":120,"tempo_parada_min":0,"motorista_email":"ana@exemplo.com"},"numero":1}],"preco_total":50}}
```

Para vários trechos, inclua os IDs em ordem na mesma requisição; a confirmação é integral. Não envie uma confirmação separada por trecho se deseja atomicidade do itinerário.

### Consultar caronas e reservas

```json
{"acao":"CONSULTAR_CARONAS","sessao":"SESSAO_ANA","dados":{}}
```

O retorno é `dados: [Carona, ...]`, com os objetos completos descritos acima. Após a confirmação, `passageiros` do trecho contém:

```json
[{"reserva_id":"R000003","passageiro":{"nome":"Bia Souza","email":"bia@exemplo.com","perfil":"PASSAGEIRO"},"assento":1}]
```

```json
{"acao":"CONSULTAR_RESERVAS","sessao":"SESSAO_PASSAGEIRO","dados":{}}
```

O retorno é `dados: [Reserva, ...]`, inclusive reservas canceladas. Para qualquer uma dessas consultas, se a conta ainda não possui registros:

```json
{"status":"SUCESSO","mensagem":"Operação concluída.","dados":[]}
```

### Cancelar reserva, trecho ou carona

```json
{"acao":"CANCELAR_RESERVA","sessao":"SESSAO_PASSAGEIRO","dados":{"id":"R000003"}}
```

```json
{"status":"SUCESSO","mensagem":"Operação concluída.","dados":{"id":"R000003","passageiro_email":"bia@exemplo.com","status":"CANCELADA","criada_em":"2099-10-01T10:15:00Z","cancelada_em":"2099-10-01T10:20:00Z","motivo":"Cancelada pelo passageiro.","assentos":[{"trecho":{"status":"ATIVA","distancia_km":100,"id":"T000002","carona_id":"C000001","origem":"Salvador","destino":"Feira de Santana","data_hora":"2099-10-01T08:00:00-03:00","assentos_livres":1,"capacidade":2,"preco":50,"tempo_viagem_min":120,"tempo_parada_min":0,"motorista_email":"ana@exemplo.com"},"numero":1}],"preco_total":50}}
```

Alternativas do motorista:

```json
{"acao":"CANCELAR_TRECHO","sessao":"SESSAO_ANA","dados":{"id":"T000002"}}
```

```json
{"acao":"CANCELAR_CARONA","sessao":"SESSAO_ANA","dados":{"id":"C000001"}}
```

Nesta oferta de um único trecho, ambas devolvem a mesma estrutura de carona cancelada:

```json
{"status":"SUCESSO","mensagem":"Operação concluída.","dados":{"id":"C000001","motorista_email":"ana@exemplo.com","rota":["Salvador","Feira de Santana"],"data_hora":"2099-10-01T08:00:00-03:00","status":"CANCELADA","trechos":[{"trecho":{"status":"CANCELADA","distancia_km":100,"id":"T000002","carona_id":"C000001","origem":"Salvador","destino":"Feira de Santana","data_hora":"2099-10-01T08:00:00-03:00","assentos_livres":2,"capacidade":2,"preco":50,"tempo_viagem_min":120,"tempo_parada_min":0,"motorista_email":"ana@exemplo.com"},"passageiros":[]}]}}
```

Em caronas maiores, cancelar um trecho mantém os demais ativos e pode retornar `PARCIALMENTE_CANCELADA`. As regras de prazo e devolução integral de reservas estão na seção de cancelamentos abaixo.

### Consultar avisos, marcar leitura e sair

Se o motorista cancela a carona enquanto a reserva ainda está ativa, o passageiro recebe um aviso consultável:

```json
{"acao":"CONSULTAR_NOTIFICACOES","sessao":"SESSAO_PASSAGEIRO","dados":{}}
```

```json
{"status":"SUCESSO","mensagem":"Operação concluída.","dados":[{"id":"N000004","reserva_id":"R000003","mensagem":"Carona C000001 cancelada pelo motorista; itinerário inteiro cancelado.","criada_em":"2099-10-01T10:20:00Z","lida":false}]}
```

```json
{"acao":"LER_NOTIFICACAO","sessao":"SESSAO_PASSAGEIRO","dados":{"id":"N000004"}}
```

```json
{"status":"SUCESSO","mensagem":"Operação concluída."}
```

```json
{"acao":"DESCONECTAR","sessao":"SESSAO_PASSAGEIRO","dados":{}}
```

```json
{"status":"SUCESSO","mensagem":"Operação concluída."}
```

### Erros e recuperação

Todos os erros de aplicação usam o mesmo envelope, por exemplo:

```json
{"status":"ERRO","mensagem":"chave já utilizada com outros dados"}
```

Podem ocorrer erros por JSON inválido, ação desconhecida, credenciais/sessão inválidas, perfil incorreto, recurso de outro usuário, campos fora dos limites, falta de vagas, conexão temporal impossível ou cancelamento após partida. Corrija os dados ou autentique novamente conforme o caso. Erros de formato em linhas completas normalmente permitem enviar outra linha na conexão; excesso de tamanho e falhas de leitura/escrita encerram o socket. Ao atingir 256 conexões, novas conexões podem ser fechadas sem resposta JSON.

Ausência de resposta não prova que a operação falhou: em publicação/confirmação, consulte o histórico ou repita com a mesma chave. Não gere outra chave automaticamente após uma resposta perdida. O cadastro não possui chave de idempotência; caso a resposta seja perdida, tente autenticar com as credenciais cadastradas.

## Uso por terminal

Na raiz, inicie `go run ./cmd/servidor`. Em outros terminais, abra os clientes interativos:

```powershell
go run ./cmd/cliente_motorista
go run ./cmd/cliente_passageiro
```

Ao criar uma conta, o cliente inicia o acesso automaticamente. Para uma conta existente, escolha **Entrar**. O identificador da sessão fica somente no estado interno do cliente.

## Execução distribuída

```powershell
docker compose build
docker compose up -d servidor
docker compose run --rm motorista
docker compose run --rm carga -clientes 100 -vagas 5
```

A construção da imagem executa testes com `-race` e `go vet` no estágio `testes`. O servidor publica a porta TCP 8080 do host. Na máquina cliente do laboratório, construa a imagem e configure o IP LAN real da máquina servidor, sem iniciar outro servidor:

```powershell
$env:VAIJUNTO_SERVIDOR = '192.168.1.10:8080'
docker compose run --rm passageiro
docker compose run --rm carga -clientes 100 -vagas 5
```

Substitua o IP pelo endereço real do laboratório e permita TCP 8080 no firewall do servidor. O nome `servidor` só resolve na rede Compose local; em hosts distintos usa-se o IP LAN e a porta publicada. Não use localhost para alcançar outra máquina.

Carga nativa: `go run ./cmd/carga -servidor 127.0.0.1:8080 -clientes 100 -vagas 5`. Cria contas e duas caronas de motoristas distintos para o teste; ao terminar, cancela as caronas. Mede apenas a fase concorrente de confirmação: duração, média, p95, vazão, confirmações, recusas e erros de transporte. Os cadastros e o histórico de teste permanecem em RAM. O processo retorna código diferente de zero se falhar a integridade ou o transporte.

## Regras de cancelamento e avisos

`CANCELAR_TRECHO` recebe `dados: {"id":"ID_DO_TRECHO"}` e exige a sessão do motorista proprietário. Cancela apenas a oferta daquele trecho; os demais trechos permanecem ativos. A carona fica `PARCIALMENTE_CANCELADA` ou `CANCELADA` se todos os seus trechos foram cancelados. Toda reserva ativa que inclui o trecho cancelado é cancelada por inteiro, devolvendo as vagas em todos os seus trechos uma única vez.

Cancelamentos do motorista, de trecho ou carona inteira, são recusados a partir da partida inicial da carona. Também são recusados quando qualquer reserva afetada já iniciou o primeiro trecho do seu itinerário, mesmo que esse primeiro trecho pertença a outro motorista. A validação ocorre sob a mesma trava de escrita, antes de mudar vagas, reservas ou notificações. Um passageiro também não pode cancelar a própria reserva depois da primeira partida do itinerário.

`CONSULTAR_NOTIFICACOES` recebe objeto vazio e retorna somente os avisos do usuário autenticado. Cada aviso contém `id`, `reserva_id`, `mensagem`, `criada_em` e `lida`. `LER_NOTIFICACAO` recebe `dados: {"id":"ID_DO_AVISO"}` e marca o aviso do próprio usuário como lido. Consultar não remove avisos; marcar leitura novamente não causa efeitos adicionais.

O servidor cria o aviso de cancelamento na mesma seção crítica que altera a reserva. O cliente passageiro consulta avisos ao entrar e retornar ao menu; a opção 5, Verificar notificações, permite ver também o histórico. Não há envio espontâneo enquanto o usuário está parado preenchendo um campo. Os avisos permanecem em RAM durante desconexões do cliente, mas se perdem ao encerrar o servidor.

No menu do motorista, a opção 5 cancela um trecho. A opção 2 mostra a quantidade atual de vagas e os passageiros por trecho. A interface não oferece escolha nem exibe número de assento: cada reserva ocupa uma vaga por trecho. Os números usados internamente servem ao controle e aos testes de ocupação.

Publicações com a mesma rota e chaves distintas criam caronas e trechos com IDs distintos. Repetir a mesma chave e os mesmos dados continua sendo uma repetição idempotente da publicação anterior.

Os menus são abertos com `go run ./cmd/cliente_motorista` e `go run ./cmd/cliente_passageiro`. A busca continua consultiva; a trava protege a confirmação e é liberada antes do envio da resposta TCP.
