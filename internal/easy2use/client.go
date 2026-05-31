package easy2use

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
	logger     *slog.Logger
}

func NewClient(baseURL, token string, logger *slog.Logger) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
	}
}

type ListSubscribersResponse struct {
	StatusCodeTip any          `json:"codigo_status_tip"`
	Results       []Subscriber `json:"results"`
}

type Subscriber struct {
	PersonType string     `json:"tipo_pessoa"`
	Document   string     `json:"cpf_cnpj"`
	Name       string     `json:"nome"`
	Contracts  []Contract `json:"contratos"`
}

type Contract struct {
	Status         string `json:"status"`
	SimCard        string `json:"sim_card"`
	ContractNumber string `json:"numero_contrato"`
	PhoneLine      string `json:"linha_contrato"`
	Plan           string `json:"plano"`
}

type LastRechargeResponse struct {
	LastRecharge  string `json:"ultima_recarga"`
	StatusCodeTip any    `json:"codigo_status_tip"`
}

type AddBalanceResponse struct {
	UserMessage   string          `json:"msg_usuario"`
	StatusCodeTip any             `json:"codigo_status_tip"`
	Americanet    json.RawMessage `json:"americanet"`
}

func (c *Client) ListSubscribers(ctx context.Context) (ListSubscribersResponse, []byte, int, error) {
	var out ListSubscribersResponse
	body, statusCode, err := c.do(ctx, http.MethodGet, "/assinantes/listar", nil)
	if err != nil {
		return out, body, statusCode, err
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return out, body, statusCode, fmt.Errorf("decode subscribers response: %w", err)
	}
	return out, body, statusCode, nil
}

func (c *Client) LastRecharge(ctx context.Context, simCard string) (LastRechargeResponse, []byte, int, error) {
	var out LastRechargeResponse
	path := fmt.Sprintf("/simcard/%s/ultima-recarga", url.PathEscape(simCard))
	body, statusCode, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return out, body, statusCode, err
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return out, body, statusCode, fmt.Errorf("decode last recharge response: %w", err)
	}
	return out, body, statusCode, nil
}

func (c *Client) AddBalance(ctx context.Context, simCard string, quantity int) (AddBalanceResponse, []byte, int, error) {
	var out AddBalanceResponse
	payload := map[string]int{"quantity": quantity}
	path := fmt.Sprintf("/simcard/%s/saldo/adicionar", url.PathEscape(simCard))
	body, statusCode, err := c.do(ctx, http.MethodPost, path, payload)
	if err != nil {
		return out, body, statusCode, err
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return out, body, statusCode, fmt.Errorf("decode add balance response: %w", err)
	}
	return out, body, statusCode, nil
}

func (c *Client) do(ctx context.Context, method string, path string, payload any) ([]byte, int, error) {
	endpoint, err := url.Parse(c.baseURL + path)
	if err != nil {
		return nil, 0, err
	}
	query := endpoint.Query()
	query.Set("user_token", c.token)
	endpoint.RawQuery = query.Encode()

	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, 0, err
		}
		body = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Error("provider request failed", "method", method, "path", path, "error", err)
		return nil, 0, err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}

	c.logger.Info("provider request completed", "method", method, "path", path, "status", resp.StatusCode, "duration_ms", time.Since(start).Milliseconds())
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return responseBody, resp.StatusCode, fmt.Errorf("provider returned status %d", resp.StatusCode)
	}
	return responseBody, resp.StatusCode, nil
}

func StatusCodeTipOK(value any) bool {
	switch v := value.(type) {
	case string:
		return v == "0"
	case float64:
		return v == 0
	case int:
		return v == 0
	default:
		return false
	}
}
