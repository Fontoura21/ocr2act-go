// Package audit mantém a trilha de auditoria das ações de revisão: um JSONL
// só de acréscimo em que cada evento guarda o hash do anterior. Alterar ou
// remover um evento quebra a cadeia a partir dele.
//
// O hash e o formato da linha são os mesmos do protótipo em Python, então uma
// trilha gravada por um é verificada pelo outro.
package audit

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Genesis é o hash anterior do primeiro evento.
var Genesis = strings.Repeat("0", 64)

// Event é uma ação de revisão humana sobre um campo.
type Event struct {
	DocID          string  `json:"doc_id"`
	DocSHA256      string  `json:"sha256_documento"`
	Field          string  `json:"campo"`
	Action         string  `json:"acao"`
	ReadValue      string  `json:"valor_lido"`
	CorrectedValue *string `json:"valor_corrigido"`
	SuggestedClass string  `json:"classe_sugerida"`
	Rule           string  `json:"regra"`
	Engine         string  `json:"motor"`
	EngineVersion  string  `json:"versao_motor"`
	RulesVersion   string  `json:"versao_regras"`
	// Preenchidos por Append.
	TS       string `json:"ts"`
	PrevHash string `json:"hash_anterior"`
	Hash     string `json:"hash"`
}

// members devolve os pares do evento na ordem em que a linha é gravada.
func (e Event) members() []member {
	var corrected any
	if e.CorrectedValue != nil {
		corrected = *e.CorrectedValue
	}
	return []member{
		{"doc_id", e.DocID}, {"sha256_documento", e.DocSHA256}, {"campo", e.Field},
		{"acao", e.Action}, {"valor_lido", e.ReadValue}, {"valor_corrigido", corrected},
		{"classe_sugerida", e.SuggestedClass}, {"regra", e.Rule}, {"motor", e.Engine},
		{"versao_motor", e.EngineVersion}, {"versao_regras", e.RulesVersion},
		{"ts", e.TS}, {"hash_anterior", e.PrevHash},
	}
}

// Log é a trilha gravada em um arquivo JSONL. É seguro para várias
// goroutines; vários processos gravando a mesma trilha não são suportados.
type Log struct {
	path string
	now  func() time.Time
	mu   sync.Mutex
}

// New abre a trilha em path. O arquivo é criado no primeiro evento.
func New(path string) *Log {
	return &Log{path: path, now: time.Now}
}

// Path é o arquivo da trilha.
func (l *Log) Path() string { return l.path }

// Append acrescenta o evento, preenchendo o horário, o hash anterior e o
// próprio hash, e devolve o evento gravado. A escrita é sincronizada com o
// disco antes de voltar.
func (l *Log) Append(e Event) (Event, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	events, err := l.read()
	if err != nil {
		return Event{}, err
	}
	e.PrevHash = Genesis
	if len(events) > 0 {
		e.PrevHash = events[len(events)-1].Hash
	}
	e.TS = l.now().UTC().Format("2006-01-02T15:04:05+00:00")
	sanitize(&e)
	members := e.members()
	e.Hash = hashOf(members)
	line := encodeObject(append(members, member{"hash", e.Hash}), false) + "\n"

	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return Event{}, fmt.Errorf("auditoria: %w", err)
	}
	if _, err := f.WriteString(line); err != nil {
		return Event{}, errors.Join(fmt.Errorf("auditoria: %w", err), f.Close())
	}
	if err := f.Sync(); err != nil {
		return Event{}, errors.Join(fmt.Errorf("auditoria: %w", err), f.Close())
	}
	return e, f.Close()
}

// Events devolve os eventos na ordem em que foram gravados.
func (l *Log) Events() ([]Event, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.read()
}

func (l *Log) read() ([]Event, error) {
	lines, err := l.lines()
	if err != nil {
		return nil, err
	}
	events := make([]Event, 0, len(lines))
	for i, line := range lines {
		var e Event
		if err := json.Unmarshal(line, &e); err != nil {
			return nil, fmt.Errorf("auditoria: evento %d: %w", i+1, err)
		}
		events = append(events, e)
	}
	return events, nil
}

// lines devolve as linhas não vazias da trilha; uma trilha inexistente está vazia.
func (l *Log) lines() ([][]byte, error) {
	data, err := os.ReadFile(l.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("auditoria: %w", err)
	}
	var out [][]byte
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), len(data)+1)
	for sc.Scan() {
		if line := bytes.TrimSpace(sc.Bytes()); len(line) > 0 {
			out = append(out, bytes.Clone(line))
		}
	}
	return out, sc.Err()
}

// Verification é o resultado da verificação da cadeia.
type Verification struct {
	OK      bool
	Events  int
	Message string
}

// Verify recalcula a cadeia inteira. Para no primeiro evento com conteúdo
// alterado ou com o hash anterior diferente do hash do evento antes dele.
func (l *Log) Verify() (Verification, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	lines, err := l.lines()
	if err != nil {
		return Verification{}, err
	}
	prev := Genesis
	for i, line := range lines {
		dec := json.NewDecoder(bytes.NewReader(line))
		dec.UseNumber() // números voltam com o texto original para o hash
		var obj map[string]any
		if err := dec.Decode(&obj); err != nil {
			return Verification{Message: fmt.Sprintf("evento %d: JSON inválido", i+1)}, nil
		}
		if obj["hash_anterior"] != prev {
			return Verification{Message: fmt.Sprintf("evento %d: encadeamento quebrado", i+1)}, nil
		}
		hash, _ := obj["hash"].(string)
		delete(obj, "hash")
		if hashOf(obj) != hash {
			return Verification{Message: fmt.Sprintf("evento %d: conteúdo alterado", i+1)}, nil
		}
		prev = hash
	}
	return Verification{OK: true, Events: len(lines), Message: fmt.Sprintf("%d evento(s) íntegros", len(lines))}, nil
}

// hashOf é o sha256 do corpo com as chaves ordenadas, como
// json.dumps(corpo, sort_keys=True, ensure_ascii=False) no Python.
func hashOf(body any) string {
	sum := sha256.Sum256([]byte(encode(body, true)))
	return hex.EncodeToString(sum[:])
}

// sanitize troca bytes que não são UTF-8 válido, que o JSON não representa.
func sanitize(e *Event) {
	for _, s := range []*string{&e.DocID, &e.DocSHA256, &e.Field, &e.Action, &e.ReadValue, &e.SuggestedClass,
		&e.Rule, &e.Engine, &e.EngineVersion, &e.RulesVersion} {
		*s = strings.ToValidUTF8(*s, string(utf8.RuneError))
	}
	if e.CorrectedValue != nil {
		v := strings.ToValidUTF8(*e.CorrectedValue, string(utf8.RuneError))
		e.CorrectedValue = &v
	}
}
