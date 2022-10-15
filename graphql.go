package graphql

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/nobody05/graphql_go_client/internal/jsonutil"
	"golang.org/x/net/context/ctxhttp"
)

// Client is a GraphQL client.
type Client struct {
	url        string // GraphQL server URL.
	httpClient *http.Client
}

// NewClient creates a GraphQL client targeting the specified GraphQL server URL.
// If httpClient is nil, then http.DefaultClient is used.
func NewClient(url string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{
		url:        url,
		httpClient: httpClient,
	}
}

// Query executes a single GraphQL query request,
// with a query derived from q, populating the response into it.
// q should be a pointer to struct that corresponds to the GraphQL schema.
func (c *Client) Query(ctx context.Context, fn string, q interface{}, variables map[string]interface{}) (map[string]interface{}, error) {
	return c.doForWbyDc(ctx, queryOperation, fn, q, variables)
}

// Mutate executes a single GraphQL mutation request,
// with a mutation derived from m, populating the response into it.
// m should be a pointer to struct that corresponds to the GraphQL schema.
func (c *Client) Mutate(ctx context.Context, m interface{}, variables map[string]interface{}) error {
	return c.do(ctx, mutationOperation, m, variables)
}

func (c *Client) doForWbyDc(ctx context.Context, op operationType, fn string, v interface{}, variables map[string]interface{}) (map[string]interface{}, error) {
	var query string
	switch op {
	case queryOperation:
		query = constructQueryNoQueryKeyword(fn, v, variables)
	case mutationOperation:
		query = constructMutation(v, variables)
	}

	var buf bytes.Buffer
	buf.WriteString(query)
	resp, err := ctxhttp.Post(ctx, c.httpClient, c.url, "application/json", &buf)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("non-200 OK status code: %v body: %q", resp.Status, body)
	}

	result, _ := ioutil.ReadAll(resp.Body)

	var resultMap map[string]interface{}
	var resultData map[string]interface{}
	err = json.Unmarshal(result, &resultMap)
	if err != nil {
		return nil, err
	}
	if err, exit := resultMap["errors"]; exit {
		errs := &errors{}
		errStr, _ := json.Marshal(err)
		err := json.Unmarshal(errStr, errs)
		if err != nil {
			return nil, err
		}
		return nil, errs
	}

	if body, exit := resultMap["data"]; exit {
		dataStr, _ := json.Marshal(body)
		json.Unmarshal(dataStr, &resultData)
		return resultData, nil
	}
	return nil, nil
}

// do executes a single GraphQL operation.
func (c *Client) do(ctx context.Context, op operationType, v interface{}, variables map[string]interface{}) error {
	var query string
	switch op {
	case queryOperation:
		query = constructQuery(v, variables)
	case mutationOperation:
		query = constructMutation(v, variables)
	}
	in := struct {
		Query     string                 `json:"query"`
		Variables map[string]interface{} `json:"variables,omitempty"`
	}{
		Query:     query,
		Variables: variables,
	}
	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(in)
	if err != nil {
		return err
	}
	resp, err := ctxhttp.Post(ctx, c.httpClient, c.url, "application/json", &buf)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("non-200 OK status code: %v body: %q", resp.Status, body)
	}
	var out struct {
		Data   *json.RawMessage
		Errors errors
		//Extensions interface{} // Unused.
	}
	err = json.NewDecoder(resp.Body).Decode(&out)
	if err != nil {
		// TODO: Consider including response body in returned error, if deemed helpful.
		return err
	}
	if out.Data != nil {
		err := jsonutil.UnmarshalGraphQL(*out.Data, v)
		if err != nil {
			// TODO: Consider including response body in returned error, if deemed helpful.
			return err
		}
	}
	if len(out.Errors) > 0 {
		return out.Errors
	}
	return nil
}

// errors represents the "errors" array in a response from a GraphQL server.
// If returned via error interface, the slice is expected to contain at least 1 element.
//
// Specification: https://facebook.github.io/graphql/#sec-Errors.
type errors []struct {
	Message   string
	Locations []struct {
		Line   int
		Column int
	}
}

// Error implements error interface.
func (e errors) Error() string {
	return e[0].Message
}

type operationType uint8

const (
	queryOperation operationType = iota
	mutationOperation
	//subscriptionOperation // Unused.
)
