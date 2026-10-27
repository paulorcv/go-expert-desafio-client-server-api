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
