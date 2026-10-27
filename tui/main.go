package main

import (
	"bufio"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	_ "modernc.org/sqlite"
)

const (
	burstSize = 5
	dbRows    = 20
)

type pane int

const (
	clientPane pane = iota
	serverPane
	txtPane
	dbPane
	paneCount
)

type (
	logMsg struct {
		pane pane
		text string
	}
	buildDoneMsg     struct{ err error }
	serverStartedMsg struct{ pid int }
	serverExitedMsg  struct{ err error }
	filesMsg         struct{ txt, db string }
	clientDoneMsg    struct {
		id      int
		output  string
		elapsed time.Duration
		err     error
	}
)

var (
	colorClient = lipgloss.Color("39")
	colorServer = lipgloss.Color("170")
	colorFile   = lipgloss.Color("214")
	colorDim    = lipgloss.Color("241")

	dimStyle   = lipgloss.NewStyle().Foreground(colorDim)
	okStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	errStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	keyStyle   = lipgloss.NewStyle().Bold(true)
	titleStyle = lipgloss.NewStyle().Bold(true)
)

// app guarda o estado compartilhado com as goroutines que acompanham os processos.
type app struct {
	root   string
	binDir string

	mu         sync.Mutex
	program    *tea.Program
	server     *exec.Cmd
	serverDone chan struct{}
}

func (a *app) send(msg tea.Msg) {
	a.mu.Lock()
	p := a.program
	a.mu.Unlock()
	if p != nil {
		p.Send(msg)
	}
}

func (a *app) build() tea.Cmd {
	return func() tea.Msg {
		for _, target := range []string{"server", "client"} {
			cmd := exec.Command("go", "build", "-o", filepath.Join(a.binDir, target), "./"+target)
			cmd.Dir = a.root
			if out, err := cmd.CombinedOutput(); err != nil {
				return buildDoneMsg{fmt.Errorf("go build ./%s: %v\n%s", target, err, out)}
			}
		}
		return buildDoneMsg{}
	}
}

func (a *app) startServer() tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command(filepath.Join(a.binDir, "server"))
		cmd.Dir = filepath.Join(a.root, "server")
		pr, pw := io.Pipe()
		cmd.Stdout = pw
		cmd.Stderr = pw
		if err := cmd.Start(); err != nil {
			return serverExitedMsg{err}
		}

		done := make(chan struct{})
		a.mu.Lock()
		a.server = cmd
		a.serverDone = done
		a.mu.Unlock()

		scanDone := make(chan struct{})
		go func() {
			defer close(scanDone)
			sc := bufio.NewScanner(pr)
			for sc.Scan() {
				a.send(logMsg{serverPane, sc.Text()})
			}
		}()
		go func() {
			err := cmd.Wait()
			pw.Close()
			<-scanDone
			a.mu.Lock()
			a.server = nil
			a.mu.Unlock()
			close(done)
			a.send(serverExitedMsg{err})
		}()

		return serverStartedMsg{cmd.Process.Pid}
	}
}

// stopServer encerra o server, se estiver rodando, e retorna um canal fechado quando ele terminar.
func (a *app) stopServer() <-chan struct{} {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.server == nil {
		return nil
	}
	a.server.Process.Signal(os.Interrupt)
	return a.serverDone
}

func (a *app) callClient(id int) tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command(filepath.Join(a.binDir, "client"))
		cmd.Dir = filepath.Join(a.root, "client")
		start := time.Now()
		out, err := cmd.CombinedOutput()
		return clientDoneMsg{id, strings.TrimRight(string(out), "\n"), time.Since(start), err}
	}
}

func (a *app) readFiles() tea.Cmd {
	return func() tea.Msg {
		return filesMsg{
			txt: readCotacaoTxt(filepath.Join(a.root, "client", "cotacao.txt")),
			db:  readCotacoesDB(filepath.Join(a.root, "server", "cotacoes.db")),
		}
	}
}

func readCotacaoTxt(path string) string {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "── arquivo ainda não existe"
	}
	if err != nil {
		return "erro ao ler arquivo: " + err.Error()
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "erro ao ler arquivo: " + err.Error()
	}
	return fmt.Sprintf("%s\n\n── modificado em %s", data, info.ModTime().Format("15:04:05"))
}

func readCotacoesDB(path string) string {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return "── banco ainda não existe"
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return "erro ao abrir banco: " + err.Error()
	}
	defer db.Close()

	var total int
	if err := db.QueryRow("SELECT COUNT(*) FROM cotacoes").Scan(&total); err != nil {
		return "erro ao consultar banco: " + err.Error()
	}

	rows, err := db.Query(`SELECT id, bid, ask, pct_change, create_date,
		strftime('%Y-%m-%d %H:%M:%S', created_at, 'localtime')
		FROM cotacoes ORDER BY id DESC LIMIT ?`, dbRows)
	if err != nil {
		return "erro ao consultar banco: " + err.Error()
	}
	defer rows.Close()

	var b strings.Builder
	fmt.Fprintf(&b, "── %d registros (exibindo os %d mais recentes)\n", total, min(total, dbRows))
	fmt.Fprintf(&b, "%-5s %-8s %-8s %-10s %-19s %-19s\n", "id", "bid", "ask", "var%", "create_date (API)", "gravado em")
	for rows.Next() {
		var id int
		var bid, ask, pct, createDate, createdAt sql.NullString
		if err := rows.Scan(&id, &bid, &ask, &pct, &createDate, &createdAt); err != nil {
			return "erro ao ler registro: " + err.Error()
		}
		fmt.Fprintf(&b, "%-5d %-8s %-8s %-10s %-19s %-19s\n", id, bid.String, ask.String, pct.String, createDate.String, createdAt.String)
	}
	if err := rows.Err(); err != nil {
		return "erro ao ler registros: " + err.Error()
	}
	return strings.TrimRight(b.String(), "\n")
}

type logView struct {
	lines []string
	vp    viewport.Model
	// nowrap mantém linhas longas inteiras (rolagem horizontal), útil para tabelas.
	nowrap bool
}

func (l *logView) append(text string) {
	atBottom := l.vp.AtBottom()
	l.lines = append(l.lines, strings.Split(text, "\n")...)
	l.refresh()
	if atBottom {
		l.vp.GotoBottom()
	}
}

func (l *logView) set(text string) {
	l.lines = strings.Split(text, "\n")
	l.refresh()
}

func (l *logView) clear() {
	l.lines = nil
	l.refresh()
}

func (l *logView) refresh() {
	styled := make([]string, len(l.lines))
	for i, line := range l.lines {
		styled[i] = colorize(line)
	}
	content := strings.Join(styled, "\n")
	if !l.nowrap {
		content = lipgloss.NewStyle().Width(l.vp.Width).Render(content)
	}
	l.vp.SetContent(content)
}

func colorize(line string) string {
	lower := strings.ToLower(line)
	switch {
	case strings.HasPrefix(line, "──"):
		return dimStyle.Render(line)
	case strings.Contains(lower, "erro"), strings.Contains(lower, "timeout"), strings.Contains(lower, "exit status"):
		return errStyle.Render(line)
	case strings.Contains(line, "Dólar:"), strings.Contains(line, "bid="), strings.Contains(line, "ouvindo"):
		return okStyle.Render(line)
	}
	return line
}

type model struct {
	app *app

	width, height int
	focus         pane
	views         [paneCount]*logView

	built       bool
	serverState string
	restarting  bool
	quitting    bool
	calls       int
	pending     int
}

func newModel(a *app) model {
	m := model{app: a, serverState: "compilando…"}
	for i := range m.views {
		m.views[i] = &logView{vp: viewport.New(0, 0)}
	}
	m.views[dbPane].nowrap = true
	return m
}

func (m model) Init() tea.Cmd {
	m.views[serverPane].append("── compilando server e client…")
	return m.app.build()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resize()
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case buildDoneMsg:
		if msg.err != nil {
			m.serverState = "falha na compilação"
			m.views[serverPane].append(msg.err.Error())
			return m, nil
		}
		m.built = true
		m.serverState = "iniciando…"
		m.views[serverPane].append("── compilado, subindo server…")
		m.views[clientPane].append("── pressione c para chamar o client")
		return m, tea.Batch(m.app.startServer(), m.app.readFiles())

	case serverStartedMsg:
		m.serverState = fmt.Sprintf("rodando (pid %d)", msg.pid)
		return m, nil

	case serverExitedMsg:
		if m.quitting {
			return m, nil
		}
		status := "server encerrado"
		if msg.err != nil {
			status = fmt.Sprintf("server encerrado: %v", msg.err)
		}
		m.views[serverPane].append("── " + status)
		if m.restarting {
			m.restarting = false
			m.serverState = "iniciando…"
			return m, m.app.startServer()
		}
		m.serverState = "parado"
		return m, nil

	case filesMsg:
		m.views[txtPane].set(msg.txt)
		m.views[dbPane].set(msg.db)
		return m, nil

	case logMsg:
		m.views[msg.pane].append(msg.text)
		return m, nil

	case clientDoneMsg:
		m.pending--
		result := "ok"
		if msg.err != nil {
			result = "falhou"
		}
		m.views[clientPane].append(fmt.Sprintf("── chamada #%d · %s · %s", msg.id, msg.elapsed.Round(time.Millisecond), result))
		if msg.output != "" {
			m.views[clientPane].append(msg.output)
		}
		if msg.err != nil {
			m.views[clientPane].append(msg.err.Error())
		}
		// Só lê o banco sem chamadas em andamento, para não disputar o lock
		// com o insert do server (timeout de 10ms).
		if m.pending == 0 {
			return m, m.app.readFiles()
		}
		return m, nil
	}
	return m, nil
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		m.quitting = true
		return m, tea.Quit

	case "c", "enter":
		if !m.built {
			return m, nil
		}
		return m, m.newCalls(1)

	case "m":
		if !m.built {
			return m, nil
		}
		return m, m.newCalls(burstSize)

	case "r":
		if !m.built || m.restarting {
			return m, nil
		}
		m.views[serverPane].append("── reiniciando server…")
		if m.app.stopServer() == nil {
			m.serverState = "iniciando…"
			return m, m.app.startServer()
		}
		m.restarting = true
		m.serverState = "reiniciando…"
		return m, nil

	case "s":
		if m.app.stopServer() != nil {
			m.views[serverPane].append("── parando server…")
		}
		return m, nil

	case "a":
		if m.pending > 0 {
			return m, nil
		}
		return m, m.app.readFiles()

	case "x":
		m.views[m.focus].clear()
		return m, nil

	case "tab":
		m.focus = (m.focus + 1) % paneCount
		return m, nil
	}

	var cmd tea.Cmd
	m.views[m.focus].vp, cmd = m.views[m.focus].vp.Update(msg)
	return m, cmd
}

func (m *model) newCalls(n int) tea.Cmd {
	cmds := make([]tea.Cmd, n)
	for i := range cmds {
		m.calls++
		m.pending++
		cmds[i] = m.app.callClient(m.calls)
	}
	return tea.Batch(cmds...)
}

// Grade 2x2: client/server em cima, cotacao.txt/cotacoes.db embaixo;
// a última linha é o rodapé de atalhos.
func (m *model) resize() {
	for i, v := range m.views {
		v.vp.Width = m.boxWidth(pane(i)) - 4         // borda + padding horizontal
		v.vp.Height = max(m.boxHeight(pane(i))-3, 1) // borda + linha de título
		v.refresh()
		if pane(i) == clientPane || pane(i) == serverPane {
			v.vp.GotoBottom()
		}
	}
}

func (m model) boxWidth(p pane) int {
	if p == clientPane || p == txtPane {
		return m.width / 2
	}
	return m.width - m.width/2
}

func (m model) boxHeight(p pane) int {
	available := m.height - 1
	top := available * 3 / 5
	if p == clientPane || p == serverPane {
		return top
	}
	return available - top
}

func (m model) View() string {
	if m.width == 0 || m.quitting {
		return ""
	}

	clientTitle := "CLIENT"
	if m.pending > 0 {
		clientTitle += fmt.Sprintf(" · %d em andamento", m.pending)
	}
	left := m.box(clientPane, colorClient, clientTitle)
	right := m.box(serverPane, colorServer, "SERVER :8080 · "+m.serverState)
	txt := m.box(txtPane, colorFile, "client/cotacao.txt")
	db := m.box(dbPane, colorFile, "server/cotacoes.db")

	return lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.JoinHorizontal(lipgloss.Top, left, right),
		lipgloss.JoinHorizontal(lipgloss.Top, txt, db),
		m.footer(),
	)
}

func (m model) box(p pane, color lipgloss.Color, title string) string {
	v := m.views[p]
	border := colorDim
	if m.focus == p {
		border = color
	}
	header := titleStyle.Foreground(color).Render(truncate(title, v.vp.Width))
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Padding(0, 1).
		Width(m.boxWidth(p) - 2).
		Height(m.boxHeight(p) - 2).
		Render(header + "\n" + v.vp.View())
}

func (m model) footer() string {
	keys := [][2]string{
		{"c", "chamar client"},
		{"m", fmt.Sprintf("%d chamadas", burstSize)},
		{"r", "reiniciar server"},
		{"s", "parar server"},
		{"a", "atualizar arquivos"},
		{"x", "limpar painel"},
		{"tab", "trocar foco"},
		{"↑↓←→", "rolar"},
		{"q", "sair"},
	}
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = keyStyle.Render(k[0]) + " " + dimStyle.Render(k[1])
	}
	return truncate(strings.Join(parts, dimStyle.Render(" · ")), m.width)
}

func truncate(s string, width int) string {
	return lipgloss.NewStyle().MaxWidth(width).Render(s)
}

func findRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("go.mod não encontrado; execute a partir da raiz do projeto")
		}
		dir = parent
	}
}

func main() {
	root, err := findRoot()
	if err != nil {
		log.Fatal(err)
	}
	binDir, err := os.MkdirTemp("", "cotacao-tui-")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(binDir)

	a := &app{root: root, binDir: binDir}
	p := tea.NewProgram(newModel(a), tea.WithAltScreen())
	a.mu.Lock()
	a.program = p
	a.mu.Unlock()

	_, runErr := p.Run()

	if done := a.stopServer(); done != nil {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			a.mu.Lock()
			if a.server != nil {
				a.server.Process.Kill()
			}
			a.mu.Unlock()
		}
	}

	if runErr != nil {
		log.Fatal(runErr)
	}
}
