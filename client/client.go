package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

const (
	serverURL     = "http://localhost:8080/cotacao"
	clientTimeout = 300 * time.Millisecond
	outputFile    = "cotacao.txt"
)

type cotacaoResponse struct {
	Bid string `json:"bid"`
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), clientTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL, nil)
	if err != nil {
		log.Fatalf("erro ao criar requisição: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			log.Fatalf("timeout de %v excedido ao aguardar o servidor: %v", clientTimeout, err)
		}
		log.Fatalf("erro ao chamar o servidor: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Fatalf("servidor retornou status %d", resp.StatusCode)
	}

	var cotacao cotacaoResponse
	if err := json.NewDecoder(resp.Body).Decode(&cotacao); err != nil {
		log.Fatalf("erro ao decodificar resposta: %v", err)
	}

	conteudo := fmt.Sprintf("Dólar: %s", cotacao.Bid)
	if err := os.WriteFile(outputFile, []byte(conteudo), 0644); err != nil {
		log.Fatalf("erro ao salvar arquivo: %v", err)
	}

	log.Println(conteudo)
}
