# Go Expert - Desafio Client-Server-API

Primeiro desafio do curso **Go Expert** da [Full Cycle](https://fullcycle.com.br).

## Objetivo

Criar dois sistemas em Go:

- `client.go`
- `server.go`

### Requisitos

- O `client.go` deve fazer uma requisição HTTP ao `server.go` solicitando a cotação do dólar.
- O `server.go` deve consumir a API `https://economia.awesomeapi.com.br/json/last/USD-BRL` e retornar o resultado no formato JSON para o cliente.
- Usando o package `context`, o `server.go` deve registrar no banco de dados SQLite cada cotação recebida:
  - timeout máximo para chamar a API de cotação: **200ms**
  - timeout máximo para persistir os dados no banco: **10ms**
- O `client.go` precisa receber do `server.go` apenas o valor atual do câmbio (campo `bid` do JSON).
  - Usando o package `context`, o `client.go` terá um timeout máximo de **300ms** para receber o resultado do `server.go`.
- Os 3 contextos devem retornar erro nos logs caso o tempo de execução seja insuficiente.
- O `client.go` deve salvar a cotação atual em um arquivo `cotacao.txt` no formato: `Dólar: {valor}`
- O endpoint do `server.go` deve ser `/cotacao` e a porta do servidor HTTP deve ser **8080**.

## Estrutura

```
.
├── server/server.go   # servidor HTTP na porta 8080 (GET /cotacao)
├── client/client.go   # cliente que salva a cotação em cotacao.txt
└── tui/main.go        # TUI com client e server lado a lado
```

O banco SQLite usa o driver `modernc.org/sqlite` (Go puro, não precisa de CGO).

## Como executar

Requer Go 1.25+.

### Modo interativo (TUI) — recomendado

Para ver client e server lado a lado num só terminal:

```bash
go run ./tui
```

O TUI compila os dois programas, sobe o server automaticamente e mostra os logs dele à direita. Cada chamada ao client aparece à esquerda, com a duração e o resultado. Ao sair, o server é encerrado.

Na parte de baixo ficam o conteúdo de `client/cotacao.txt` e os 20 registros mais recentes de `server/cotacoes.db`. Os dois são atualizados quando as chamadas em andamento terminam.

| Tecla | Ação |
|-------|------|
| `c` / `enter` | chama o client |
| `m` | dispara 5 chamadas simultâneas |
| `r` | reinicia o server |
| `s` | para o server |
| `a` | atualiza os painéis de cotacao.txt e cotacoes.db |
| `x` | limpa o painel em foco |
| `tab` | alterna o foco entre os painéis |
| `↑` `↓` `←` `→` `pgup` `pgdn` | rola o painel em foco |
| `q` / `ctrl+c` | sai |

### Modo manual (dois terminais)

Em um terminal, suba o servidor (o banco `cotacoes.db` é criado dentro de `server/`):

```bash
cd server
go run server.go
```

Em outro terminal, execute o cliente (o arquivo `cotacao.txt` é criado dentro de `client/`):

```bash
cd client
go run client.go
cat cotacao.txt   # Dólar: 5.3416
```

Para consultar as cotações gravadas:

```bash
sqlite3 server/cotacoes.db "SELECT id, bid, created_at FROM cotacoes;"
```

Se algum dos timeouts (200ms na API, 10ms no banco, 300ms no cliente) for excedido, o erro é registrado no log.
