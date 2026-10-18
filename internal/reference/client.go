package reference

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client consulta a API de referência por HTTP, para rodar o pipeline contra
// o serviço de referência em vez da base local.
//
// A API devolve 404 tanto para registro ausente quanto para CPF ambíguo, então
// pelo cliente um CPF ambíguo aparece como não encontrado. O resultado final é
// o mesmo: o documento vai para revisão.
type Client struct {
	base string
	http *http.Client
}

// NewClient cria o cliente para baseURL (por exemplo http://localhost:8080).
// Com httpClient nil, usa um cliente com tempo limite de 5 segundos.
func NewClient(baseURL string, httpClient *http.Client) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("referência: URL inválida %q", baseURL)
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Second}
	}
	return &Client{base: strings.TrimSuffix(u.String(), "/"), http: httpClient}, nil
}

// ByMatricula consulta GET /ref/registros/matricula/{matricula}.
func (c *Client) ByMatricula(ctx context.Context, matricula string) ([]Entry, error) {
	return c.get(ctx, "matricula", matricula)
}

// ByCPF consulta GET /ref/registros/cpf/{cpf}.
func (c *Client) ByCPF(ctx context.Context, cpf string) ([]Entry, error) {
	return c.get(ctx, "cpf", cpf)
}

func (c *Client) get(ctx context.Context, key, value string) ([]Entry, error) {
	endpoint := c.base + "/ref/registros/" + key + "/" + url.PathEscape(value)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body := io.LimitReader(resp.Body, 1<<20)
	switch resp.StatusCode {
	case http.StatusOK:
		var e Entry
		if err := json.NewDecoder(body).Decode(&e); err != nil {
			return nil, fmt.Errorf("referência: resposta inválida: %w", err)
		}
		return []Entry{e}, nil
	case http.StatusNotFound:
		_, _ = io.Copy(io.Discard, body) // permite reaproveitar a conexão
		return nil, nil
	default:
		return nil, fmt.Errorf("referência: %s respondeu %s", endpoint, resp.Status)
	}
}
