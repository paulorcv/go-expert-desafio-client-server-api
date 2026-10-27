package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	_ "modernc.org/sqlite"
)

const (
	cotacaoURL    = "https://economia.awesomeapi.com.br/json/last/USD-BRL"
	apiTimeout    = 200 * time.Millisecond
	dbTimeout     = 10 * time.Millisecond
	serverAddress = ":8080"
	dbFile        = "cotacoes.db"
)

type Cotacao struct {
	Code       string `json:"code"`
	Codein     string `json:"codein"`
	Name       string `json:"name"`
	High       string `json:"high"`
	Low        string `json:"low"`
	VarBid     string `json:"varBid"`
	PctChange  string `json:"pctChange"`
	Bid        string `json:"bid"`
	Ask        string `json:"ask"`
	Timestamp  string `json:"timestamp"`
	CreateDate string `json:"create_date"`
}

type apiResponse struct {
	USDBRL Cotacao `json:"USDBRL"`
}

type cotacaoResponse struct {
	Bid string `json:"bid"`
}

func main() {
	db, err := sql.Open("sqlite", dbFile)
	if err != nil {
		log.Fatalf("erro ao abrir o banco de dados: %v", err)
	}
	defer db.Close()

	if err := createTable(db); err != nil {
		log.Fatalf("erro ao criar a tabela: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /cotacao", cotacaoHandler(db))

	log.Printf("servidor ouvindo em %s", serverAddress)
	log.Fatal(http.ListenAndServe(serverAddress, mux))
}

func createTable(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS cotacoes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		code TEXT,
		codein TEXT,
		name TEXT,
		high TEXT,
		low TEXT,
		var_bid TEXT,
		pct_change TEXT,
		bid TEXT,
		ask TEXT,
		timestamp TEXT,
		create_date TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	return err
}

func cotacaoHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cotacao, err := buscaCotacao(r.Context())
		if err != nil {
			log.Printf("erro ao buscar cotação: %v", err)
			http.Error(w, "erro ao buscar cotação", http.StatusGatewayTimeout)
			return
		}

		if err := salvaCotacao(r.Context(), db, cotacao); err != nil {
			log.Printf("erro ao salvar cotação: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(cotacaoResponse{Bid: cotacao.Bid}); err != nil {
			log.Printf("erro ao escrever resposta: %v", err)
		}
	}
}

func buscaCotacao(parent context.Context) (*Cotacao, error) {
	ctx, cancel := context.WithTimeout(parent, apiTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cotacaoURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("timeout de %v excedido ao chamar a API: %w", apiTimeout, err)
		}
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API retornou status %d", resp.StatusCode)
	}

	var data apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	return &data.USDBRL, nil
}

func salvaCotacao(parent context.Context, db *sql.DB, c *Cotacao) error {
	ctx, cancel := context.WithTimeout(parent, dbTimeout)
	defer cancel()

	_, err := db.ExecContext(ctx,
		`INSERT INTO cotacoes (code, codein, name, high, low, var_bid, pct_change, bid, ask, timestamp, create_date)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.Code, c.Codein, c.Name, c.High, c.Low, c.VarBid, c.PctChange, c.Bid, c.Ask, c.Timestamp, c.CreateDate,
	)
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("timeout de %v excedido ao persistir no banco: %w", dbTimeout, err)
	}
	return err
}
