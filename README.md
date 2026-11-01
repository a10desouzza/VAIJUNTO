# Consórcios — Problema 2: Caronas Compartilhadas

Projeto da disciplina TEC502 — Concorrência e Conectividade. O consórcio reúne **VaiJunto**, **Minas** e **São Paulo**, com um servidor independente por empresa.

Este guia usa **Linux e Bash**, como no laboratório. O cenário principal executa um contêiner Docker em cada um de três computadores diferentes.

## Sumário

- [Como funciona](#como-funciona)
- [1. Preparar os três computadores](#1-preparar-os-três-computadores)
- [2. Identificar os endereços e as portas](#2-identificar-os-endereços-e-as-portas)
- [3. Compilar a imagem Docker](#3-compilar-a-imagem-docker)
- [4. Iniciar uma empresa em cada computador](#4-iniciar-uma-empresa-em-cada-computador)
- [5. Verificar a comunicação](#5-verificar-a-comunicação)
- [6. Executar os clientes](#6-executar-os-clientes)
- [7. Demonstrar uma viagem entre empresas](#7-demonstrar-uma-viagem-entre-empresas)
- [8. Executar o simulador de carga](#8-executar-o-simulador-de-carga)
- [9. Demonstrar falhas de comunicação](#9-demonstrar-falhas-de-comunicação)
- [Backup local e recuperação](#backup-local-e-recuperação)
- [Portas ocupadas e seleção automática](#portas-ocupadas-e-seleção-automática)
- [Executar sem Docker](#executar-sem-docker)
- [Três empresas no mesmo computador](#três-empresas-no-mesmo-computador)
- [Executar no Windows com PowerShell](#executar-no-windows-com-powershell)
- [Testes automatizados](#testes-automatizados)
- [Arquitetura e organização do código](#arquitetura-e-organização-do-código)
- [Garantias e limites](#garantias-e-limites)
- [Problemas comuns](#problemas-comuns)

## Como funciona

O motorista publica caronas somente na própria empresa. O passageiro também se conecta apenas à sua empresa, mas pode encontrar e reservar itinerários que combinem trechos das outras duas.

| Comunicação | Transporte | Porta usada nos exemplos |
|---|---|---|
| Cliente → sua empresa | TCP com JSON delimitado por linha (NDJSON) | 8080 |
| Empresa → outras empresas | API REST com HTTP e JSON | 8081 |

As empresas trocam ofertas para consulta e metadados para coordenar transações. Cada uma mantém suas contas, caronas e assentos em RAM e salva um arquivo local para recuperação. Os arquivos ficam separados por empresa. Não existe banco de dados, armazenamento compartilhado entre empresas ou coordenador fixo.

## 1. Preparar os três computadores

Em cada computador:

1. Tenha Docker instalado e o serviço em execução.
2. Disponibilize a mesma versão do código do projeto.
3. Abra um terminal na pasta que contém `Dockerfile` e `go.mod`.
4. Confira o Docker:

```bash
docker version
```

Os exemplos pressupõem que seu usuário pode executar Docker. Caso o laboratório exija `sudo`, use-o nos comandos Docker. Ao iniciar o servidor com `sudo`, passe o token explicitamente, conforme indicado adiante.

Go instalado no computador é opcional para o caminho com Docker: a imagem compila o servidor, os clientes e o simulador. Para executar diretamente com Go, o projeto declara **Go 1.27.0** em `go.mod`.

## 2. Identificar os endereços e as portas

Descubra o IPv4 de cada computador:

```bash
hostname -I
ip -4 addr
```

Escolha o endereço da interface conectada à rede do laboratório. Não use `127.0.0.1`, o endereço de uma interface Docker ou um IP de exemplo sem substituí-lo.

Este guia usará:

| Computador | Empresa | Identificador no código | IPv4 de exemplo |
|---|---|---|---|
| 1 | VaiJunto | `vaijunto` | `192.168.1.10` |
| 2 | Minas | `minas` | `192.168.1.11` |
| 3 | São Paulo | `sao-paulo` | `192.168.1.12` |

**Substitua esses três IPs em todos os comandos pelos IPs reais.** O identificador de cada empresa precisa ser o mesmo em `-empresa` e nas listas `-pares`.

Os três computadores podem usar 8080 e 8081 simultaneamente: endereços IP diferentes não disputam a mesma porta. O conflito ocorre quando outro processo já ocupa aquela porta no mesmo endereço do mesmo computador.

Confira as portas locais:

```bash
ss -ltn '( sport = :8080 or sport = :8081 )'
```

Os computadores precisam se alcançar pela rede. Libere as portas publicadas no firewall conforme a configuração do laboratório; HTTP não abre portas bloqueadas automaticamente.

## 3. Compilar a imagem Docker

Execute em **cada computador**, na pasta do projeto:

```bash
docker build -t vaijunto:local .
```

O Dockerfile executa testes com detector de corridas, executa `go vet` e compila os quatro programas. Se a construção falhar, corrija a falha antes de iniciar os contêineres.

Defina o **mesmo token** nos três computadores:

```bash
export VAIJUNTO_TOKEN_CONSORCIO='token-demonstracao-consorcio-2026'
```

Esse é um token de demonstração com mais de 16 caracteres. Ele autentica a comunicação entre empresas; não é a senha de um passageiro. Cada terminal tem seu próprio ambiente: repita o `export` ao abrir outro terminal que precise do token.

## 4. Iniciar uma empresa em cada computador

Execute somente o comando da empresa correspondente a cada computador. Os comandos abaixo deixam o servidor em primeiro plano, mostrando os logs. Use outro terminal para executar clientes.

### Computador 1 — VaiJunto

```bash
docker run --rm --name vaijunto -v vaijunto-dados:/dados \
  -p 8080:8080 -p 8081:8081 \
  -e VAIJUNTO_TOKEN_CONSORCIO="$VAIJUNTO_TOKEN_CONSORCIO" \
  vaijunto:local \
  -empresa vaijunto \
  -endereco :8080 \
  -rest :8081 \
  -pares 'minas=http://192.168.1.11:8081,sao-paulo=http://192.168.1.12:8081'
```

### Computador 2 — Minas

```bash
docker run --rm --name minas -v minas-dados:/dados \
  -p 8080:8080 -p 8081:8081 \
  -e VAIJUNTO_TOKEN_CONSORCIO="$VAIJUNTO_TOKEN_CONSORCIO" \
  vaijunto:local \
  -empresa minas \
  -endereco :8080 \
  -rest :8081 \
  -pares 'vaijunto=http://192.168.1.10:8081,sao-paulo=http://192.168.1.12:8081'
```

### Computador 3 — São Paulo

```bash
docker run --rm --name sao-paulo -v sao-paulo-dados:/dados \
  -p 8080:8080 -p 8081:8081 \
  -e VAIJUNTO_TOKEN_CONSORCIO="$VAIJUNTO_TOKEN_CONSORCIO" \
  vaijunto:local \
  -empresa sao-paulo \
  -endereco :8080 \
  -rest :8081 \
  -pares 'vaijunto=http://192.168.1.10:8081,minas=http://192.168.1.11:8081'
```

O terminal deve mostrar a identidade da empresa e os endereços TCP e REST. Inicie as três empresas antes da demonstração de reservas distribuídas.

Nos comandos Bash, a barra `\` precisa ser o último caractere da linha, sem espaços depois dela.

### Significado das opções

| Opção | Função |
|---|---|
| `--name` | Nome local do contêiner, usado em comandos como `docker pause` |
| `-p 8080:8080` | Publica a porta TCP do contêiner no computador |
| `-p 8081:8081` | Publica a API REST |
| `-empresa` | Identidade da empresa dentro do consórcio |
| `-endereco :8080` | Escuta TCP em todas as interfaces do contêiner |
| `-rest :8081` | Escuta HTTP em todas as interfaces do contêiner |
| `-pares` | Identidades e URLs REST das outras duas empresas |
| `-timeout-rest` | Prazo de uma chamada a outra empresa; padrão de 3 segundos |
| `-backup` | Arquivo privado de recuperação; padrão `dados/EMPRESA.json` fora do Docker |

No laboratório com computadores distintos, os pares usam os IPs dos **computadores**, não nomes de serviços de uma rede Docker local.

## 5. Verificar a comunicação

Em um terminal Linux com o token definido, consulte as três empresas:

```bash
curl --fail --show-error \
  -H "Authorization: Bearer $VAIJUNTO_TOKEN_CONSORCIO" \
  http://192.168.1.10:8081/distribuido/saude

curl --fail --show-error \
  -H "Authorization: Bearer $VAIJUNTO_TOKEN_CONSORCIO" \
  http://192.168.1.11:8081/distribuido/saude

curl --fail --show-error \
  -H "Authorization: Bearer $VAIJUNTO_TOKEN_CONSORCIO" \
  http://192.168.1.12:8081/distribuido/saude
```

Cada resposta contém `empresa_id` e `geracao`. A geração identifica o histórico de coordenação da empresa e é preservada quando ela reinicia com seu backup íntegro.

Peça ao VaiJunto para consultar o consórcio:

```bash
curl --fail --show-error \
  -H "Authorization: Bearer $VAIJUNTO_TOKEN_CONSORCIO" \
  http://192.168.1.10:8081/consorcio/trechos
```

Antes de publicar caronas, é esperado que `trechos` esteja vazio. A lista `empresas_indisponiveis` deve estar vazia quando todos os participantes responderem.

## 6. Executar os clientes

Os programas de cliente já estão na imagem. Abra outro terminal e escolha o servidor da empresa do usuário.

Motorista, com escolha da empresa no início:

```bash
docker run --rm -it \
  --entrypoint /bin/cliente_motorista \
  vaijunto:local \
  -vaijunto 192.168.1.10:8080 \
  -minas 192.168.1.11:8080 \
  -sao-paulo 192.168.1.12:8080
```

Passageiro, com a mesma escolha:

```bash
docker run --rm -it \
  --entrypoint /bin/cliente_passageiro \
  vaijunto:local \
  -vaijunto 192.168.1.10:8080 \
  -minas 192.168.1.11:8080 \
  -sao-paulo 192.168.1.12:8080
```

Ao iniciar, escolha `1 - VAIJUNTO`, `2 - Minas` ou `3 - São Paulo`. Depois, crie a conta ou entre na empresa escolhida. Para acesso direto, `-servidor IP:porta` continua disponível e dispensa essa seleção.

Em uma execução com Go no mesmo computador, sem informar endereços, o menu usa `localhost:7001`, `localhost:7002` e `localhost:7003`, as portas do Compose. Abra os clientes em terminais separados:

```bash
go run ./cmd/cliente_motorista
go run ./cmd/cliente_passageiro
```

Também é possível configurar os endereços por `VAIJUNTO_SERVIDOR_VAIJUNTO`, `VAIJUNTO_SERVIDOR_MINAS` e `VAIJUNTO_SERVIDOR_SAO_PAULO`. São endereços TCP completos, como `192.168.1.10:8080`.

O endereço precisa ser o IP do computador servidor. Dentro de um contêiner de cliente, `localhost` apontaria para o próprio contêiner de cliente.

No menu, crie uma conta ou autentique-se. O cadastro inicia a sessão automaticamente. A conta pertence somente à empresa em que foi criada. A senha deve ter de 8 a 128 caracteres; neste protótipo ela fica visível no terminal e é guardada em texto puro, inclusive no backup local. Use contas e senhas de demonstração.

A sessão dura 15 minutos. Fechar uma conexão TCP não encerra a sessão; sair pelo menu revoga a sessão.

## 7. Demonstrar uma viagem entre empresas

Crie um motorista em cada empresa e publique uma oferta em cada servidor. Use uma data futura igual para todos os trechos. O exemplo usa **01/10/2099**, aceito pelo sistema para evitar que o horário da apresentação torne as ofertas indisponíveis.

| Empresa | Origem | Destino | Partida | Viagem | Preço por pessoa | Vagas |
|---|---|---|---|---|---|---|
| VaiJunto | Salvador | Feira de Santana | 08:00 | 60 minutos | 20,00 | 1 |
| São Paulo | Feira de Santana | Vitória da Conquista | 09:30 | 180 minutos | 40,00 | 1 |
| Minas | Vitória da Conquista | Montes Claros | 13:00 | 240 minutos | 60,00 | 1 |

Informe duas cidades por oferta e uma distância positiva. Como cada oferta termina na segunda cidade, não há parada intermediária. O horário informado pela interface usa o fuso `-03:00`.

Depois:

1. Abra o cliente passageiro apontando para o **VaiJunto**.
2. Crie uma conta de passageiro nessa empresa.
3. Escolha buscar itinerário.
4. Informe `Salvador`, `Montes Claros` e `01/10/2099`.
5. Selecione a opção com os três trechos e confirme.
6. Confira a reserva em **Minhas reservas**.
7. Nos três clientes motorista, consulte as caronas e a ocupação dos respectivos trechos.

O passageiro não precisa criar contas nas outras empresas. O servidor de origem combina as ofertas e coordena a reserva. Uma reserva global só aparece como `ATIVA` depois da aplicação em todos os donos dos trechos. Se a decisão ou aplicação ainda estiver pendente, a interface mostra `PENDENTE`.

Com uma vaga por trecho, uma segunda tentativa para a mesma viagem não deve confirmar outro passageiro nos mesmos assentos. Cancele a reserva antes da partida para demonstrar a devolução das vagas nas três empresas.

## 8. Executar o simulador de carga

O simulador cria usuários e ofertas próprios, faz buscas e disputa reservas a partir dos três servidores. Execute de um computador com a imagem e acesso à rede:

```bash
docker run --rm \
  --entrypoint /bin/carga \
  vaijunto:local \
  -servidores 192.168.1.10:8080,192.168.1.11:8080,192.168.1.12:8080 \
  -clientes 100 \
  -vagas 5
```

As três URLs acima são endereços **TCP dos clientes**, sem `http://`.

A saída é JSON com quantidade de buscas, confirmações, recusas, pendências, erros de transporte, duração, latências, requisições por segundo e indicador `integridade`.

A integridade compara as transações confirmadas com a ocupação de cada empresa e verifica ausência de assentos duplicados ou segmentos confirmados fora dos resultados esperados. Tentativas concorrentes podem ser abortadas sem consumir todas as vagas; não se exige uma taxa artificial de confirmações. O simulador retorna erro se encontrar inconsistência, pendências não resolvidas ou falhas de transporte.

Limites: de 1 a 200 clientes e de 1 a 100 vagas por trecho. O token REST não precisa ser passado ao simulador: ele atua como clientes TCP, e cada servidor usa sua própria configuração REST.

## 9. Demonstrar falhas de comunicação

Para simular indisponibilidade **preservando a RAM**, no computador da Minas execute em outro terminal:

```bash
docker pause minas
```

Durante a pausa:

- Consultas podem retornar apenas as empresas disponíveis.
- Reservas inteiramente locais das outras empresas continuam funcionando.
- Uma reserva que precisa de um trecho da Minas não pode ser concluída sem a participação necessária.
- Operações já iniciadas são tratadas pelo protocolo de decisão e recuperação; timeout sozinho não libera vagas.

Para retomar o mesmo processo:

```bash
docker unpause minas
```

Acompanhe os logs e consulte as reservas novamente. O ciclo de recuperação roda periodicamente; operações sem decisão iniciam recuperação após um intervalo de suspeita de aproximadamente 10 segundos. O tempo total depende dos prazos de rede e da disponibilidade de uma maioria.

Os testes automatizados injetam falhas nos pontos de preparação e decisão para verificar os cenários mais difíceis de reproduzir manualmente.

`docker stop` e `docker kill` encerram o processo. Ao iniciar novamente com o mesmo volume, identidade e token, a empresa carrega o backup e retoma a recuperação das transações. Os usuários precisam entrar novamente; suas contas e reservas permanecem.

## Backup local e recuperação

Cada empresa grava somente seu estado e seus registros de coordenação. No Docker, o arquivo é `/dados/estado.json`. Os exemplos usam um volume diferente para cada empresa: `vaijunto-dados`, `minas-dados` e `sao-paulo-dados`. O Compose usa os volumes separados `dados-a`, `dados-b` e `dados-c`, associados ao nome do projeto.

Fora do Docker, o padrão é `dados/EMPRESA.json`. Use `-backup caminho/arquivo.json` ou `VAIJUNTO_BACKUP` para escolher outro arquivo. Uma execução com `-backup=` funciona apenas em RAM. Execute somente um processo por arquivo de backup.

Depois de cada escrita, o servidor salva uma fotografia do estado. As etapas REST de preparação, promessa, aceitação e aplicação também são salvas antes da resposta de sucesso. O arquivo inclui vagas ocupadas e preparadas, chaves de repetição, reservas, notificações, relógio lógico e decisões. Sessões de login são temporárias e não são restauradas.

A gravação usa um arquivo temporário, sincronização com o disco e substituição do arquivo anterior. A abertura valida versão, empresa e checksum. A recuperação distribuída exige os mesmos participantes e token. Um arquivo inválido impede a inicialização e é preservado para análise; o servidor não começa vazio silenciosamente.

Para testar uma queda real no cenário de um computador, no Linux ou no Windows:

```bash
docker compose -p laboratorio-arthur -f compose.consorcio.yaml kill -s SIGKILL empresa-b
docker compose -p laboratorio-arthur -f compose.consorcio.yaml start empresa-b
docker compose -p laboratorio-arthur -f compose.consorcio.yaml logs --tail 50 empresa-b
```

Entre novamente na conta da Minas e consulte as caronas e reservas. Durante a queda, uma reserva confirmada continua aparecendo no servidor de origem, com a empresa indisponível indicada. Uma preparação ainda sem decisão será resolvida pelo protocolo após a recuperação; ela não é liberada apenas por timeout.

Em computadores separados, um contêiner iniciado com `--rm` será removido após a queda: repita seu comando `docker run`, preservando o mesmo volume. O backup protege contra encerramento do processo, desde que o arquivo atual permaneça íntegro. Perder o disco ou restaurar manualmente uma cópia antiga não equivale a essa recuperação.

`docker compose down` preserva os volumes. Para apagar o ambiente de demonstração e seus dados, use `docker compose -p laboratorio-arthur -f compose.consorcio.yaml down -v` somente quando quiser começar do zero nas três empresas.

## Portas ocupadas e seleção automática

### Alterar a porta publicada no Docker

Se a porta REST 8081 estiver ocupada no computador da Minas, use neste computador:

```bash
-p 9001:8081
```

Mantenha `-rest :8081` dentro do contêiner. Nos comandos das **outras duas empresas**, troque a URL da Minas por:

```text
minas=http://192.168.1.11:9001
```

Se trocar a publicação TCP para `-p 9000:8080`, os clientes da Minas devem usar `192.168.1.11:9000`. Essa mudança não afeta as URLs REST.

### Pedir portas externas livres ao Docker

Na execução de uma empresa, substitua as publicações fixas por:

```bash
-p 0:8080 -p 0:8081
```

Depois descubra as portas publicadas, por exemplo para a Minas:

```bash
docker port minas 8080/tcp
docker port minas 8081/tcp
```

Use a porta TCP efetiva nos clientes e a REST efetiva nos `-pares` dos outros servidores. A aplicação não tem descoberta automática dos novos endereços. Para preparar uma apresentação com portas aleatórias, descubra as três portas REST e inicie novamente os servidores com essas portas publicadas explicitamente e os pares corrigidos. Preserve os volumes de backup ao recriar os contêineres.

Para o cenário com três computadores, portas conhecidas e livres simplificam a configuração. Não é necessário alterar `rede.go`: flags e variáveis de ambiente substituem os padrões.

## Executar sem Docker

Esta opção serve para desenvolvimento. Para o cenário solicitado pelo enunciado, use um contêiner por computador.

Com Go 1.27.0 e o código em cada máquina, defina o token comum e execute o comando correspondente:

```bash
export VAIJUNTO_TOKEN_CONSORCIO='token-demonstracao-consorcio-2026'
```

VaiJunto:

```bash
go run ./cmd/servidor \
  -empresa vaijunto -endereco :8080 -rest :8081 \
  -pares 'minas=http://192.168.1.11:8081,sao-paulo=http://192.168.1.12:8081'
```

Minas:

```bash
go run ./cmd/servidor \
  -empresa minas -endereco :8080 -rest :8081 \
  -pares 'vaijunto=http://192.168.1.10:8081,sao-paulo=http://192.168.1.12:8081'
```

São Paulo:

```bash
go run ./cmd/servidor \
  -empresa sao-paulo -endereco :8080 -rest :8081 \
  -pares 'vaijunto=http://192.168.1.10:8081,minas=http://192.168.1.11:8081'
```

Clientes e carga:

```bash
go run ./cmd/cliente_motorista -servidor 192.168.1.10:8080
go run ./cmd/cliente_passageiro -servidor 192.168.1.10:8080
go run ./cmd/carga -servidores 192.168.1.10:8080,192.168.1.11:8080,192.168.1.12:8080 -clientes 100 -vagas 5
```

O padrão dos clientes em `internal/configuracao/rede.go` é `localhost:8080`. Isso só aponta para o servidor local quando o cliente roda diretamente nesse computador. Para outra máquina, informe seu IP.

Para escolher uma porta TCP livre automaticamente:

```bash
go run ./cmd/servidor -endereco :0
```

A porta efetiva aparece no terminal; informe-a em `-servidor`. Esse comando isolado inicia somente o atendimento local. Para habilitar o consórcio, inclua também identidade, REST, token e pares. `-rest :0` também escolhe uma porta livre, que precisa ser comunicada aos demais.

## Três empresas no mesmo computador

Este roteiro executa três servidores isolados no mesmo computador, usando `compose.consorcio.yaml`. Não é necessário instalar Go para os passos com Docker. A apresentação principal continua sendo em computadores diferentes.

| Serviço Docker | Empresa representada | TCP no computador | REST no computador |
|---|---|---|---|
| empresa-a | VaiJunto | localhost:7001 | http://localhost:8001 |
| empresa-b | Minas | localhost:7002 | http://localhost:8002 |
| empresa-c | São Paulo | localhost:7003 | http://localhost:8003 |

Os nomes internos desse Compose são `empresa-a`, `empresa-b` e `empresa-c`. Os registros e respostas mostrarão essas identidades. O menu dos clientes apresenta VAIJUNTO, Minas e São Paulo e usa os endereços internos já definidos no Compose. Escolha 1, 2 ou 3 conforme a conta que deseja usar. Execute todos os comandos na pasta do projeto; use o mesmo nome de projeto `laboratorio-arthur` em todos eles.

**Passo 1 — Iniciar as três empresas.** Com o Docker em execução:

```bash
export VAIJUNTO_TOKEN_CONSORCIO='token-demonstracao-consorcio-2026'
docker compose -p laboratorio-arthur -f compose.consorcio.yaml up --build -d
docker compose -p laboratorio-arthur -f compose.consorcio.yaml ps
```

Confira se os três serviços estão em execução. Acompanhe os logs quando necessário:

```bash
docker compose -p laboratorio-arthur -f compose.consorcio.yaml logs --tail=50
```

**Passo 2 — Conferir os catálogos e a comunicação.**

```bash
curl --fail --show-error -H "Authorization: Bearer $VAIJUNTO_TOKEN_CONSORCIO" http://localhost:8001/consorcio/trechos
```

Antes de publicar ofertas, `trechos` deve estar vazio. `empresas_indisponiveis` deve estar vazio se as outras duas APIs responderem. Se as portas externas foram alteradas, ajuste o endereço da consulta.

**Passo 3 — Abrir os motoristas.** Abra três terminais diferentes e execute uma linha em cada um:

```bash
# VaiJunto
docker compose -p laboratorio-arthur -f compose.consorcio.yaml exec empresa-a /bin/cliente_motorista

# Minas
docker compose -p laboratorio-arthur -f compose.consorcio.yaml exec empresa-b /bin/cliente_motorista

# São Paulo
docker compose -p laboratorio-arthur -f compose.consorcio.yaml exec empresa-c /bin/cliente_motorista
```

Aqui `exec` abre o cliente dentro de um contêiner. O menu usa os nomes internos `empresa-a:8080`, `empresa-b:8080` e `empresa-c:8080`, configurados no Compose, sem depender das portas publicadas no computador.

Crie uma conta de motorista em cada empresa e publique as ofertas do exemplo da seção 7: Salvador → Feira no VaiJunto, Feira → Vitória da Conquista na empresa São Paulo e Vitória da Conquista → Montes Claros na Minas. Use a mesma data futura, horários compatíveis e uma vaga por trecho.

**Passo 4 — Abrir os passageiros.** Para um passageiro do VaiJunto:

```bash
docker compose -p laboratorio-arthur -f compose.consorcio.yaml exec empresa-a /bin/cliente_passageiro
```

Crie a conta, busque Salvador → Montes Claros na data publicada e confirme o itinerário. Confira a reserva em Minhas reservas e a ocupação nos três motoristas.

Para testar outra origem de requisições, abra outro terminal e conecte um passageiro da Minas:

```bash
docker compose -p laboratorio-arthur -f compose.consorcio.yaml exec empresa-b /bin/cliente_passageiro
```

Com a única vaga já confirmada, a segunda reserva não deve confirmar os mesmos assentos. Para disputar a vaga simultaneamente, deixe dois passageiros com resultados de busca obtidos antes da confirmação e tente confirmar nos dois terminais. Use o simulador para concorrência automatizada.

Cancele a primeira reserva pelo passageiro e confira a devolução nos três motoristas. Depois faça nova reserva e teste o cancelamento de uma oferta pelo motorista, verificando o cancelamento da viagem inteira e o aviso ao passageiro.

**Passo 5 — Executar a carga nas três empresas.** O simulador cria contas e caronas próprias:

```bash
docker compose -p laboratorio-arthur -f compose.consorcio.yaml exec -T empresa-a /bin/carga -servidores empresa-a:8080,empresa-b:8080,empresa-c:8080 -clientes 100 -vagas 5
```

Os nomes acima são resolvidos na rede interna do Docker. Confira o JSON de métricas, especialmente `integridade`, `pendentes` e `erros_transporte`.

**Passo 6 — Testar a indisponibilidade de uma empresa preservando a RAM.** Pause a Minas:

```bash
docker compose -p laboratorio-arthur -f compose.consorcio.yaml pause empresa-b
```

Faça uma busca no passageiro do VaiJunto e observe a indicação de empresa indisponível. Uma viagem nova que exige a Minas não deve ser confirmada sem ela. Para testar independência, publique e reserve também uma carona inteiramente local no VaiJunto.

Retome o mesmo processo:

```bash
docker compose -p laboratorio-arthur -f compose.consorcio.yaml unpause empresa-b
```

Consulte as reservas e as vagas novamente. Para falhas durante pontos específicos de uma reserva, use os testes automatizados de injeção de falhas; pausar manualmente antes de iniciar uma reserva não reproduz todos esses pontos.

**Passo 7 — Executar os testes automatizados, caso Go esteja instalado no computador.**

```bash
go test ./...
go test ./internal/servidor -run 'TestConsorcio|TestBackup|TestConsulta' -v -count=1 -timeout=120s
go vet ./...
```

A construção da imagem também executa os testes. Os testes automatizados usam suas próprias instâncias temporárias e não modificam os dados dos contêineres da demonstração.

**Passo 8 — Encerrar o ambiente.** Saia dos menus e execute:

```bash
docker compose -p laboratorio-arthur -f compose.consorcio.yaml down
```

Esse comando encerra os três servidores e preserva os volumes de backup. Ao executar novamente o passo 1, os dados serão carregados; entre novamente nas contas. Para começar do zero nas três empresas, acrescente `-v` ao comando `down`.

**Portas ocupadas no teste local.** Para pedir portas externas livres, configure antes de iniciar:

```bash
export VAIJUNTO_TCP_A=0 VAIJUNTO_TCP_B=0 VAIJUNTO_TCP_C=0
export VAIJUNTO_REST_A=0 VAIJUNTO_REST_B=0 VAIJUNTO_REST_C=0
docker compose -p laboratorio-arthur -f compose.consorcio.yaml up --build -d
docker compose -p laboratorio-arthur -f compose.consorcio.yaml port empresa-a 8080
docker compose -p laboratorio-arthur -f compose.consorcio.yaml port empresa-a 8081
```

Os clientes e a carga executados com `exec` continuam usando as mesmas portas internas. Apenas consultas e clientes executados fora dos contêineres precisam das portas publicadas reais. Para voltar aos valores padrão em outro teste, remova essas variáveis do ambiente antes de iniciar. Não recrie os contêineres durante uma demonstração cujos dados deseja preservar.

## Executar no Windows com PowerShell

Esta é uma alternativa para desenvolvimento ou execução fora do laboratório. **Linux continua sendo o caminho principal da apresentação.** A arquitetura, os IPs de exemplo, as regras de reserva e o backup local são os mesmos descritos acima.

### 1. Preparar o ambiente e descobrir os IPs

Tenha o Docker Desktop em execução, configurado para **contêineres Linux**, pois o Dockerfile gera executáveis Linux. Abra o PowerShell na pasta do projeto e confira:

```powershell
docker version
ipconfig
```

No `ipconfig`, encontre o IPv4 da interface conectada à rede. Substitua os IPs de exemplo dos três computadores nos comandos seguintes:

| Empresa | Identificador | IP de exemplo |
|---|---|---|
| VaiJunto | `vaijunto` | `192.168.1.10` |
| Minas | `minas` | `192.168.1.11` |
| São Paulo | `sao-paulo` | `192.168.1.12` |

As portas TCP 8080 e REST 8081 precisam estar acessíveis no firewall do Windows e na rede usada. Se estiverem ocupadas localmente, use o procedimento de troca de portas explicado na seção principal.

### 2. Compilar e definir o token

Execute em cada computador Windows:

```powershell
docker build -t vaijunto:local .
$env:VAIJUNTO_TOKEN_CONSORCIO = 'token-demonstracao-consorcio-2026'
```

Use o mesmo token nas três empresas. Repita a definição de `$env:VAIJUNTO_TOKEN_CONSORCIO` nos novos terminais que precisarem desse valor. No PowerShell, a continuação de linha usa o acento grave `` ` ``, e não a barra invertida do Bash. Ele precisa ser o último caractere da linha, sem espaços depois.

### 3. Iniciar a empresa correspondente

Execute apenas o comando da empresa daquele computador. Deixe o terminal aberto e use outros terminais para os clientes.

**Computador do VaiJunto:**

```powershell
docker run --rm --name vaijunto -v vaijunto-dados:/dados `
  -p 8080:8080 -p 8081:8081 `
  -e "VAIJUNTO_TOKEN_CONSORCIO=$env:VAIJUNTO_TOKEN_CONSORCIO" `
  vaijunto:local `
  -empresa vaijunto `
  -endereco :8080 -rest :8081 `
  -pares 'minas=http://192.168.1.11:8081,sao-paulo=http://192.168.1.12:8081'
```

**Computador da Minas:**

```powershell
docker run --rm --name minas -v minas-dados:/dados `
  -p 8080:8080 -p 8081:8081 `
  -e "VAIJUNTO_TOKEN_CONSORCIO=$env:VAIJUNTO_TOKEN_CONSORCIO" `
  vaijunto:local `
  -empresa minas `
  -endereco :8080 -rest :8081 `
  -pares 'vaijunto=http://192.168.1.10:8081,sao-paulo=http://192.168.1.12:8081'
```

**Computador da empresa São Paulo:**

```powershell
docker run --rm --name sao-paulo -v sao-paulo-dados:/dados `
  -p 8080:8080 -p 8081:8081 `
  -e "VAIJUNTO_TOKEN_CONSORCIO=$env:VAIJUNTO_TOKEN_CONSORCIO" `
  vaijunto:local `
  -empresa sao-paulo `
  -endereco :8080 -rest :8081 `
  -pares 'vaijunto=http://192.168.1.10:8081,minas=http://192.168.1.11:8081'
```

### 4. Verificar a comunicação

Em outro PowerShell, com o token definido:

```powershell
$cabecalhos = @{ Authorization = "Bearer $env:VAIJUNTO_TOKEN_CONSORCIO" }
Invoke-RestMethod 'http://192.168.1.10:8081/distribuido/saude' -Headers $cabecalhos
Invoke-RestMethod 'http://192.168.1.11:8081/distribuido/saude' -Headers $cabecalhos
Invoke-RestMethod 'http://192.168.1.12:8081/distribuido/saude' -Headers $cabecalhos
Invoke-RestMethod 'http://192.168.1.10:8081/consorcio/trechos' -Headers $cabecalhos
```

Para conferir se uma porta remota aceita conexões:

```powershell
Test-NetConnection 192.168.1.11 -Port 8081
```

Esse teste verifica a conexão TCP; somente a chamada REST autenticada verifica a resposta da aplicação.

### 5. Executar os clientes e demonstrar a viagem

Clientes com escolha entre as três empresas, em terminais separados:

```powershell
docker run --rm -it --entrypoint /bin/cliente_motorista vaijunto:local `
  -vaijunto 192.168.1.10:8080 -minas 192.168.1.11:8080 -sao-paulo 192.168.1.12:8080
docker run --rm -it --entrypoint /bin/cliente_passageiro vaijunto:local `
  -vaijunto 192.168.1.10:8080 -minas 192.168.1.11:8080 -sao-paulo 192.168.1.12:8080
```

Escolha VAIJUNTO, Minas ou São Paulo no menu inicial. Substitua os três IPs pelos endereços reais do laboratório. Para configurar pelas variáveis do PowerShell:

```powershell
$env:VAIJUNTO_SERVIDOR_VAIJUNTO = '192.168.1.10:8080'
$env:VAIJUNTO_SERVIDOR_MINAS = '192.168.1.11:8080'
$env:VAIJUNTO_SERVIDOR_SAO_PAULO = '192.168.1.12:8080'
go run ./cmd/cliente_passageiro
```

As variáveis do terminal são usadas pela execução com Go; em `docker run`, use as três opções mostradas ou passe as variáveis com `-e`. O mesmo exemplo Salvador → Feira de Santana → Vitória da Conquista → Montes Claros continua disponível na seção principal.

### 6. Executar a carga e simular indisponibilidade

```powershell
docker run --rm --entrypoint /bin/carga vaijunto:local `
  -servidores '192.168.1.10:8080,192.168.1.11:8080,192.168.1.12:8080' `
  -clientes 100 -vagas 5
```

No computador da Minas, execute os comandos abaixo em momentos separados: primeiro pause, observe o comportamento nos demais servidores e depois retome.

```powershell
docker pause minas
docker unpause minas
```

A pausa preserva a RAM. Após encerrar o processo, o backup no volume permite recuperar o estado, como no Linux. Consulte a seção de backup para testar `kill` seguido de `start` no Compose.

### 7. Alternativa sem Docker

Com Go 1.27.0 instalado, defina o token e execute apenas a linha do servidor correspondente a cada computador:

```powershell
$env:VAIJUNTO_TOKEN_CONSORCIO = 'token-demonstracao-consorcio-2026'

# VaiJunto
go run ./cmd/servidor -empresa vaijunto -endereco :8080 -rest :8081 -pares 'minas=http://192.168.1.11:8081,sao-paulo=http://192.168.1.12:8081'

# Minas
go run ./cmd/servidor -empresa minas -endereco :8080 -rest :8081 -pares 'vaijunto=http://192.168.1.10:8081,sao-paulo=http://192.168.1.12:8081'

# São Paulo
go run ./cmd/servidor -empresa sao-paulo -endereco :8080 -rest :8081 -pares 'vaijunto=http://192.168.1.10:8081,minas=http://192.168.1.11:8081'
```

Em outros terminais, os clientes e a carga também podem ser executados diretamente:

```powershell
go run ./cmd/cliente_motorista -servidor 192.168.1.10:8080
go run ./cmd/cliente_passageiro -servidor 192.168.1.10:8080
go run ./cmd/carga -servidores '192.168.1.10:8080,192.168.1.11:8080,192.168.1.12:8080' -clientes 100 -vagas 5
```

A opção `-endereco :0` também escolhe uma porta livre no Windows. Use a porta mostrada no terminal ao conectar os clientes. Para REST dinâmico, atualize os pares com o endereço efetivo.

### 8. Testar as três empresas no mesmo computador Windows

Este roteiro executa três servidores isolados no mesmo computador, usando `compose.consorcio.yaml`. Não é necessário instalar Go para os passos com Docker. A apresentação principal continua sendo em computadores diferentes.

| Serviço Docker | Empresa representada | TCP no computador | REST no computador |
|---|---|---|---|
| empresa-a | VaiJunto | localhost:7001 | http://localhost:8001 |
| empresa-b | Minas | localhost:7002 | http://localhost:8002 |
| empresa-c | São Paulo | localhost:7003 | http://localhost:8003 |

Os nomes internos desse Compose são `empresa-a`, `empresa-b` e `empresa-c`. Os registros e respostas mostrarão essas identidades. O menu dos clientes apresenta VAIJUNTO, Minas e São Paulo e usa os endereços internos já definidos no Compose. Escolha 1, 2 ou 3 conforme a conta que deseja usar. Execute todos os comandos na pasta do projeto; use o mesmo nome de projeto `laboratorio-arthur` em todos eles.

**Passo 1 — Iniciar as três empresas.** Com o Docker em execução:

```powershell
$env:VAIJUNTO_TOKEN_CONSORCIO = 'token-demonstracao-consorcio-2026'
docker compose -p laboratorio-arthur -f compose.consorcio.yaml up --build -d
docker compose -p laboratorio-arthur -f compose.consorcio.yaml ps
```

Confira se os três serviços estão em execução. Acompanhe os logs quando necessário:

```powershell
docker compose -p laboratorio-arthur -f compose.consorcio.yaml logs --tail=50
```

**Passo 2 — Conferir os catálogos e a comunicação.**

```powershell
$cabecalhos = @{ Authorization = "Bearer $env:VAIJUNTO_TOKEN_CONSORCIO" }
Invoke-RestMethod 'http://localhost:8001/consorcio/trechos' -Headers $cabecalhos
```

Antes de publicar ofertas, `trechos` deve estar vazio. `empresas_indisponiveis` deve estar vazio se as outras duas APIs responderem. Se as portas externas foram alteradas, ajuste o endereço da consulta.

**Passo 3 — Abrir os motoristas.** Abra três terminais diferentes e execute uma linha em cada um:

```powershell
# VaiJunto
docker compose -p laboratorio-arthur -f compose.consorcio.yaml exec empresa-a /bin/cliente_motorista

# Minas
docker compose -p laboratorio-arthur -f compose.consorcio.yaml exec empresa-b /bin/cliente_motorista

# São Paulo
docker compose -p laboratorio-arthur -f compose.consorcio.yaml exec empresa-c /bin/cliente_motorista
```

Aqui `exec` abre o cliente dentro de um contêiner. O menu usa os nomes internos `empresa-a:8080`, `empresa-b:8080` e `empresa-c:8080`, configurados no Compose, sem depender das portas publicadas no computador.

Crie uma conta de motorista em cada empresa e publique as ofertas do exemplo da seção 7: Salvador → Feira no VaiJunto, Feira → Vitória da Conquista na empresa São Paulo e Vitória da Conquista → Montes Claros na Minas. Use a mesma data futura, horários compatíveis e uma vaga por trecho.

**Passo 4 — Abrir os passageiros.** Para um passageiro do VaiJunto:

```powershell
docker compose -p laboratorio-arthur -f compose.consorcio.yaml exec empresa-a /bin/cliente_passageiro
```

Crie a conta, busque Salvador → Montes Claros na data publicada e confirme o itinerário. Confira a reserva em Minhas reservas e a ocupação nos três motoristas.

Para testar outra origem de requisições, abra outro terminal e conecte um passageiro da Minas:

```powershell
docker compose -p laboratorio-arthur -f compose.consorcio.yaml exec empresa-b /bin/cliente_passageiro
```

Com a única vaga já confirmada, a segunda reserva não deve confirmar os mesmos assentos. Para disputar a vaga simultaneamente, deixe dois passageiros com resultados de busca obtidos antes da confirmação e tente confirmar nos dois terminais. Use o simulador para concorrência automatizada.

Cancele a primeira reserva pelo passageiro e confira a devolução nos três motoristas. Depois faça nova reserva e teste o cancelamento de uma oferta pelo motorista, verificando o cancelamento da viagem inteira e o aviso ao passageiro.

**Passo 5 — Executar a carga nas três empresas.** O simulador cria contas e caronas próprias:

```powershell
docker compose -p laboratorio-arthur -f compose.consorcio.yaml exec -T empresa-a /bin/carga -servidores empresa-a:8080,empresa-b:8080,empresa-c:8080 -clientes 100 -vagas 5
```

Os nomes acima são resolvidos na rede interna do Docker. Confira o JSON de métricas, especialmente `integridade`, `pendentes` e `erros_transporte`.

**Passo 6 — Testar a indisponibilidade de uma empresa preservando a RAM.** Pause a Minas:

```powershell
docker compose -p laboratorio-arthur -f compose.consorcio.yaml pause empresa-b
```

Faça uma busca no passageiro do VaiJunto e observe a indicação de empresa indisponível. Uma viagem nova que exige a Minas não deve ser confirmada sem ela. Para testar independência, publique e reserve também uma carona inteiramente local no VaiJunto.

Retome o mesmo processo:

```powershell
docker compose -p laboratorio-arthur -f compose.consorcio.yaml unpause empresa-b
```

Consulte as reservas e as vagas novamente. Para falhas durante pontos específicos de uma reserva, use os testes automatizados de injeção de falhas; pausar manualmente antes de iniciar uma reserva não reproduz todos esses pontos.

**Passo 7 — Executar os testes automatizados, caso Go esteja instalado no computador.**

```powershell
go test ./...
go test ./internal/servidor -run 'TestConsorcio|TestBackup|TestConsulta' -v -count=1 -timeout=120s
go vet ./...
```

A construção da imagem também executa os testes. Os testes automatizados usam suas próprias instâncias temporárias e não modificam os dados dos contêineres da demonstração.

**Passo 8 — Encerrar o ambiente.** Saia dos menus e execute:

```powershell
docker compose -p laboratorio-arthur -f compose.consorcio.yaml down
```

Esse comando encerra os três servidores e preserva os volumes de backup. Ao executar novamente o passo 1, os dados serão carregados; entre novamente nas contas. Para começar do zero nas três empresas, acrescente `-v` ao comando `down`.

**Portas ocupadas no teste local.** Para pedir portas externas livres, configure antes de iniciar:

```powershell
$env:VAIJUNTO_TCP_A = $env:VAIJUNTO_TCP_B = $env:VAIJUNTO_TCP_C = "0"
$env:VAIJUNTO_REST_A = $env:VAIJUNTO_REST_B = $env:VAIJUNTO_REST_C = "0"
docker compose -p laboratorio-arthur -f compose.consorcio.yaml up --build -d
docker compose -p laboratorio-arthur -f compose.consorcio.yaml port empresa-a 8080
docker compose -p laboratorio-arthur -f compose.consorcio.yaml port empresa-a 8081
```

Os clientes e a carga executados com `exec` continuam usando as mesmas portas internas. Apenas consultas e clientes executados fora dos contêineres precisam das portas publicadas reais. Para voltar aos valores padrão em outro teste, remova essas variáveis do ambiente antes de iniciar. Não recrie os contêineres durante uma demonstração cujos dados deseja preservar.

## Testes automatizados

Os testes são implementados exclusivamente em Go. A pasta `testes` reúne os testes que usam a API pública dos pacotes. Os testes que precisam acessar funções ou estados privados ficam junto ao código em `internal/cliente` e `internal/servidor`, sem tornar esses detalhes públicos apenas para testá-los. Os testes do consórcio iniciam suas próprias instâncias temporárias, sem scripts Bash ou PowerShell. O simulador também é um programa Go em `cmd/carga`.

Na pasta do projeto, com Go instalado:

```bash
go test ./...
go vet ./...
```

Para os testes do consórcio, incluindo carga TCP e injeção de falhas:

```bash
go test ./internal/servidor -run 'TestConsorcio|TestBackup|TestConsulta' -v -count=1 -timeout=120s
```

Para executar com detector de corridas no Linux, tenha compilador C instalado e suporte CGO:

```bash
CGO_ENABLED=1 go test -race ./...
```

Os testes usam portas locais escolhidas pelo sistema. Não precisam ocupar 8080 ou 8081 e não dependem dos servidores da apresentação em execução. O Dockerfile também executa o detector de corridas na construção da imagem.

A suíte cobre busca entre empresas, disputa pela última vaga a partir de três origens, idempotência, cancelamento global, queda de comunicação, ausência de maioria e recuperação dos backups após recriar os três estados locais. A recuperação é verificada com transação preparada, decisão escolhida e reserva confirmada. Também cobre arquivo danificado e conexão ociosa sem bloquear a fila.

## Arquitetura e organização do código

| Local | Responsabilidade |
|---|---|
| `cmd/servidor` | Configuração e inicialização TCP/REST |
| `cmd/cliente_motorista` | Entrada do cliente motorista |
| `cmd/cliente_passageiro` | Entrada do cliente passageiro |
| `cmd/carga` | Entrada do simulador local e distribuído |
| `internal/configuracao/rede.go` | Endereços TCP, identidades e participantes REST |
| `internal/cliente` | `menu.go`: configuração, acesso e navegação; `motorista.go` e `passageiro.go`: operações; `entrada.go`: formulários; `tcp.go`: transporte |
| `internal/protocolo` | Tipos das mensagens e validações |
| `internal/consorcio/rest.go` | Catálogos REST e agrupamento dos trechos por empresa |
| `internal/servidor/gerenciador.go` | Estado local e travas |
| `internal/servidor/fila.go` | Filas de leitura e escrita com um trabalhador TCP |
| `internal/servidor/backup.go` | Gravação e restauração do arquivo privado |
| `internal/servidor/caronas.go` | Ofertas e exploração dos itinerários |
| `internal/servidor/transacoes.go` | Preparação, confirmação e aborto locais |
| `internal/servidor/distribuido.go` | Tipos de coordenação, aplicação e recuperação |
| `internal/servidor/rest_consorcio.go` | Chamadas REST entre empresas |
| `internal/servidor/consenso.go` | Promessas, cédulas e decisão por maioria |
| `internal/servidor/itinerario_distribuido.go` | Integração da busca, reservas e cancelamentos entre empresas |
| `internal/carga/carga.go` | Cenários de carga local e distribuída, métricas e verificação de integridade |
| `testes` | Testes pela API pública: integração, menus, comunicação REST, configuração e validação |

O TCP possui duas filas, com até 128 mensagens de leitura e 128 de escrita, e um trabalhador que alterna a preferência entre elas. Leitores de rede aguardam os sockets sem ocupar esse trabalhador. O limite de transporte é de 256 conexões simultâneas; fila cheia recebe resposta de erro. Há outras goroutines para rede, REST e recuperação: o programa não executa inteiro em uma única goroutine. O buffer TCP inicial é de 4 KiB, com limite de mensagem de 1 MiB.

Os trechos têm travas próprias, adquiridas em ordem de ID. A confirmação local verifica os trechos antes de registrar a reserva. A preparação distribuída distingue assentos preparados de confirmados; confirmar não desconta a mesma vaga novamente.

As empresas são preparadas em ordem consistente e conflitos remotos são recusados sem espera por outros assentos. A precedência sobre uma vaga é a ordem de admissão no servidor dono, registrada por contador local; não é uma promessa de ordem global de envio das mensagens.

A decisão conjunta usa consenso de valor único por transação, implementado no código: cédulas com contador lógico e identificador da empresa, promessas, aceitação por maioria e certificados. O proponente é escolhido por operação, sem servidor central fixo.

A especificação das requisições, respostas, códigos HTTP e sequência das mensagens está em [internal/consorcio/API.md](internal/consorcio/API.md). O contrato TCP está em [internal/protocolo/PROTOCOLO.md](internal/protocolo/PROTOCOLO.md).

## Garantias e limites

- Cada empresa é responsável por alterar suas próprias vagas.
- As referências de trechos incluem a empresa, evitando confundir IDs locais iguais.
- Pedidos repetidos não duplicam reservas nem devolvem vagas mais de uma vez.
- Timeout indica resultado desconhecido; ele não autoriza liberar uma preparação sozinho.
- Uma decisão distribuída exige uma maioria de duas das três testemunhas.
- A confirmação ao passageiro depende da aplicação em todos os donos dos trechos.
- Reservas inteiramente locais continuam independentes das outras empresas.
- Sem maioria, operações distribuídas podem permanecer pendentes para preservar consistência.
- Um processo reiniciado com backup íntegro recupera seus dados, sua geração e as promessas de voto.
- Sem backup, uma geração nova não volta a votar em transações da geração perdida.
- Os backups são locais e não replicam as bases das empresas. Perder o arquivo ou o disco da empresa impede essa recuperação.

Salvar uma fotografia completa a cada escrita simplifica a recuperação e acrescenta custo de disco. Um trabalhador TCP também serializa as operações; isso controla a concorrência, mas não garante maior vazão. O roteiro de apresentação explica as decisões e os limites em [docs/APRESENTACAO.md](docs/APRESENTACAO.md).

## Problemas comuns

| Problema | O que conferir |
|---|---|
| Docker não responde | Serviço Docker ativo e permissão do usuário |
| Porta já ocupada | `ss -ltn`, outra publicação de porta ou seleção automática |
| Nome do contêiner já existe | `docker ps -a`; confira se já existe um servidor daquela empresa em execução |
| HTTP 401 | Mesmo token nas empresas e cabeçalho `Authorization` correto no curl |
| Outra empresa não responde | IP real, porta REST publicada, firewall e conectividade entre computadores |
| Empresa aparece indisponível | Identificador em `-pares`, token e API REST da empresa |
| Cliente não conecta | Use a porta TCP; em contêiner de cliente, informe o IP do computador servidor |
| Itinerário não aparece | Cidades, data, conexões de horário, vagas e disponibilidade das empresas |
| Reserva aparece pendente | Consulte novamente; confira se há maioria e se os donos dos trechos estão respondendo |
| Senha recusada | De 8 a 128 caracteres e cadastro na empresa correta |
| Estado sumiu após reiniciar | Mesmo volume ou caminho do backup, identidade e token; sem o arquivo não há restauração |

Para encerrar a demonstração, use Ctrl+C ou `docker stop NOME`. Contêineres com `--rm` são removidos, mas os volumes nomeados de cada empresa permanecem para a próxima execução.


### Erro no laboratorio: `too many colons in address`

Nos clientes, informe somente `IP:porta TCP publicada`. Nao use
`172.16.103.7:8081:8080`: isso mistura duas portas no mesmo endereco.
Com os servidores publicados com `-p 8080:8080 -p 8081:8081`, use:

```bash
docker run --rm -it --entrypoint /bin/cliente_motorista vaijunto:local \
  -vaijunto 172.16.103.9:8080 \
  -minas 172.16.103.8:8080 \
  -sao-paulo 172.16.103.7:8080
```

Para passageiro, troque o entrypoint por `/bin/cliente_passageiro`.
A porta 8081 e da API REST entre empresas; nao e usada pelo cliente TCP.
Se publicou TCP com `-p 9000:8080`, o cliente deve usar `IP:9000`.
Confira no computador de cada empresa com `docker port NOME_DO_CONTAINER 8080/tcp`.
O mapeamento `-p PORTA_EXTERNA:PORTA_INTERNA` pertence ao comando do
servidor Docker; ele nao faz parte do endereco passado ao cliente.
Depois de atualizar o codigo, reconstrua a imagem com
`docker build --target runtime -t vaijunto:local .` em cada computador.
