package httpapi

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"video-record/internal/auth"
	"video-record/internal/household"
	"video-record/internal/integrations"
	"video-record/internal/integrations/tmdb"
	"video-record/internal/media"
	"video-record/internal/records"
	statsdomain "video-record/internal/stats"
	"video-record/internal/storage"
	syncdomain "video-record/internal/sync"
)

type openAPIContract struct {
	OpenAPI string `yaml:"openapi"`
	Servers []struct {
		URL string `yaml:"url"`
	} `yaml:"servers"`
	Paths      map[string]map[string]yaml.Node `yaml:"paths"`
	Components struct {
		Schemas map[string]struct {
			Required   []string             `yaml:"required"`
			Properties map[string]yaml.Node `yaml:"properties"`
		} `yaml:"schemas"`
	} `yaml:"components"`
}

type openAPIOperation struct {
	OperationID string                `yaml:"operationId"`
	Security    []map[string][]string `yaml:"security"`
	Parameters  []struct {
		Ref      string `yaml:"$ref"`
		Name     string `yaml:"name"`
		In       string `yaml:"in"`
		Required bool   `yaml:"required"`
	} `yaml:"parameters"`
	RequestBody *struct {
		Required bool `yaml:"required"`
		Content  map[string]struct {
			Schema struct {
				Ref string `yaml:"$ref"`
			} `yaml:"schema"`
		} `yaml:"content"`
	} `yaml:"requestBody"`
	Responses map[string]struct {
		Headers map[string]yaml.Node `yaml:"headers"`
		Content map[string]struct {
			Schema struct {
				Ref string `yaml:"$ref"`
			} `yaml:"schema"`
		} `yaml:"content"`
	} `yaml:"responses"`
}

var protectedWriteRoutes = []struct {
	Method string
	Path   string
}{
	{http.MethodPut, "/api/v1/records/{mediaID}/rounds/current"},
	{http.MethodPost, "/api/v1/records/{mediaID}/rounds/current/rewatch"},
	{http.MethodPut, "/api/v1/records/{mediaID}/tags"},
	{http.MethodPost, "/api/v1/collections"},
	{http.MethodPost, "/api/v1/collections/{collectionID}/items"},
	{http.MethodPut, "/api/v1/collections/{collectionID}/items"},
	{http.MethodPost, "/api/v1/integrations/accounts"},
	{http.MethodDelete, "/api/v1/integrations/accounts/{accountID}"},
	{http.MethodPost, "/api/v1/data/import"},
	{http.MethodPost, "/api/v1/records/{mediaID}/progress"},
	{http.MethodPost, "/api/v1/backups"},
	{http.MethodPost, "/api/v1/restore"},
	{http.MethodPost, "/api/v1/sync/candidates/{candidateID}/confirm"},
	{http.MethodPost, "/api/v1/sync/candidates/{candidateID}/rematch"},
	{http.MethodPost, "/api/v1/sync/candidates/{candidateID}/ignore"},
	{http.MethodPost, "/api/v1/sync/candidates/{candidateID}/custom"},
	{http.MethodPost, "/api/v1/household/members"},
	{http.MethodPost, "/api/v1/household/members/{memberID}/reset-password"},
	{http.MethodPost, "/api/v1/household/members/{memberID}/deactivate"},
	{http.MethodPut, "/api/v1/household/records/{mediaID}/sharing"},
	{http.MethodPost, "/api/v1/media/tmdb/{mediaType}/{externalID}"},
	{http.MethodPost, "/api/v1/media/custom"},
	{http.MethodPost, "/api/v1/media/{id}/tmdb/{mediaType}/{externalID}"},
}

func TestContractDocumentsEveryRegisteredAPIRoute(t *testing.T) {
	router, _ := newFullContractRouter(t)
	routes, ok := router.(chi.Routes)
	require.True(t, ok)
	registered := make([]string, 0)
	require.NoError(t, chi.Walk(routes, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if strings.HasPrefix(route, "/api/v1") {
			registered = append(registered, method+" "+route)
		}
		return nil
	}))
	sort.Strings(registered)

	contract := readOpenAPIContract(t)
	require.Equal(t, "3.1.0", contract.OpenAPI)
	require.Len(t, contract.Servers, 1)
	require.Equal(t, "/", contract.Servers[0].URL)
	documented := make([]string, 0)
	for path, pathItem := range contract.Paths {
		for method, operation := range pathItem {
			normalized := strings.ToUpper(method)
			if !isHTTPMethod(normalized) {
				continue
			}
			var definition struct {
				Summary   string               `yaml:"summary"`
				Responses map[string]yaml.Node `yaml:"responses"`
			}
			require.NoError(t, operation.Decode(&definition), "%s %s", normalized, path)
			require.NotEmpty(t, definition.Summary, "%s %s needs a summary", normalized, path)
			require.NotEmpty(t, definition.Responses, "%s %s needs responses", normalized, path)
			documented = append(documented, normalized+" "+path)
		}
	}
	sort.Strings(documented)
	require.Equal(t, registered, documented)

	problem, ok := contract.Components.Schemas["ProblemDetails"]
	require.True(t, ok)
	require.ElementsMatch(t, []string{"type", "title", "status", "code", "requestId"}, problem.Required)
	for _, property := range problem.Required {
		_, exists := problem.Properties[property]
		require.True(t, exists, "ProblemDetails property %s is missing", property)
	}
}

func TestContractDefinesCursorETagAndProtectedWriteShapes(t *testing.T) {
	contract := readOpenAPIContract(t)
	cursor, ok := contract.Components.Schemas["CursorPage"]
	require.True(t, ok)
	require.ElementsMatch(t, []string{"items", "nextCursor"}, cursor.Required)
	for _, property := range cursor.Required {
		_, exists := cursor.Properties[property]
		require.True(t, exists, "CursorPage property %s is missing", property)
	}

	library := decodeOpenAPIOperation(t, contract, http.MethodGet, "/api/v1/library")
	require.Equal(t,
		"#/components/schemas/CursorPage",
		library.Responses["200"].Content["application/json"].Schema.Ref,
	)
	record := decodeOpenAPIOperation(t, contract, http.MethodGet, "/api/v1/records/{mediaID}")
	_, documentsETag := record.Responses["200"].Headers["ETag"]
	require.True(t, documentsETag, "GET record response must document ETag")
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		round := decodeOpenAPIOperation(t, contract, method, "/api/v1/records/{mediaID}/rounds/current")
		_, documentsETag := round.Responses["200"].Headers["ETag"]
		require.True(t, documentsETag, "%s current round response must document ETag", method)
	}
	for _, response := range []struct {
		Method string
		Path   string
		Status string
	}{
		{http.MethodPut, "/api/v1/records/{mediaID}/tags", "204"},
		{http.MethodGet, "/api/v1/records/{mediaID}/tags", "200"},
		{http.MethodGet, "/api/v1/records/{mediaID}/progress", "200"},
		{http.MethodPost, "/api/v1/records/{mediaID}/progress", "200"},
		{http.MethodPut, "/api/v1/household/records/{mediaID}/sharing", "200"},
		{http.MethodGet, "/api/v1/household/records/{mediaID}/sharing", "200"},
	} {
		operation := decodeOpenAPIOperation(t, contract, response.Method, response.Path)
		_, documentsETag := operation.Responses[response.Status].Headers["ETag"]
		require.True(t, documentsETag, "%s %s response must document ETag", response.Method, response.Path)
	}

	for _, route := range protectedWriteRoutes {
		operation := decodeOpenAPIOperation(t, contract, route.Method, route.Path)
		require.Contains(t, operation.Responses, "default", "%s %s needs Problem Details", route.Method, route.Path)
		hasIdempotencyKey := false
		hasCSRFToken := false
		for _, parameter := range operation.Parameters {
			if parameter.Ref == "#/components/parameters/IdempotencyKey" {
				hasIdempotencyKey = true
			}
			if parameter.Ref == "#/components/parameters/CSRFToken" {
				hasCSRFToken = true
			}
		}
		require.True(t, hasIdempotencyKey, "%s %s must document Idempotency-Key", route.Method, route.Path)
		require.True(t, hasCSRFToken, "%s %s must document X-CSRF-Token", route.Method, route.Path)
	}
	logout := decodeOpenAPIOperation(t, contract, http.MethodPost, "/api/v1/auth/logout")
	require.NotNil(t, logout.Security)
	require.Empty(t, logout.Security)
	for _, parameter := range logout.Parameters {
		require.NotEqual(t, "#/components/parameters/CSRFToken", parameter.Ref)
	}
}

func TestContractProvidesConcreteGeneratedTypesAndRealFileMedia(t *testing.T) {
	contract := readOpenAPIContract(t)
	require.NotContains(t, contract.Components.Schemas, "JSONValue")
	requestBodies := map[string]struct{}{
		http.MethodPost + " /api/v1/setup/admin":                                 {},
		http.MethodPost + " /api/v1/auth/login":                                  {},
		http.MethodPost + " /api/v1/backups":                                     {},
		http.MethodPost + " /api/v1/restore":                                     {},
		http.MethodPost + " /api/v1/collections":                                 {},
		http.MethodPost + " /api/v1/collections/{collectionID}/items":            {},
		http.MethodPost + " /api/v1/data/import":                                 {},
		http.MethodPut + " /api/v1/records/{mediaID}/rounds/current":             {},
		http.MethodPost + " /api/v1/records/{mediaID}/rounds/current/rewatch":    {},
		http.MethodPut + " /api/v1/records/{mediaID}/tags":                       {},
		http.MethodPost + " /api/v1/records/{mediaID}/progress":                  {},
		http.MethodPost + " /api/v1/sync/candidates/{candidateID}/confirm":       {},
		http.MethodPost + " /api/v1/sync/candidates/{candidateID}/rematch":       {},
		http.MethodPost + " /api/v1/sync/candidates/{candidateID}/ignore":        {},
		http.MethodPost + " /api/v1/sync/candidates/{candidateID}/custom":        {},
		http.MethodPost + " /api/v1/household/members":                           {},
		http.MethodPost + " /api/v1/household/members/{memberID}/reset-password": {},
		http.MethodPost + " /api/v1/household/members/{memberID}/deactivate":     {},
		http.MethodPut + " /api/v1/household/records/{mediaID}/sharing":          {},
		http.MethodPost + " /api/v1/media/custom":                                {},
	}
	queryParameters := map[string][]string{
		http.MethodGet + " /api/v1/calendar":        {"filter", "month", "timezone"},
		http.MethodGet + " /api/v1/data/export":     {"format"},
		http.MethodGet + " /api/v1/library":         {"cursor", "limit", "status"},
		http.MethodGet + " /api/v1/media/search":    {"q"},
		http.MethodGet + " /api/v1/stats":           {"timezone"},
		http.MethodGet + " /api/v1/sync/candidates": {"status"},
		http.MethodGet + " /api/v1/tmdb/search":     {"q"},
	}

	for path, pathItem := range contract.Paths {
		for method := range pathItem {
			normalized := strings.ToUpper(method)
			if !isHTTPMethod(normalized) {
				continue
			}
			operation := decodeOpenAPIOperation(t, contract, normalized, path)
			require.NotEmpty(t, operation.OperationID, "%s %s needs operationId", normalized, path)
			key := normalized + " " + path
			if _, needsBody := requestBodies[key]; needsBody {
				require.NotNil(t, operation.RequestBody, "%s needs requestBody", key)
				require.True(t, operation.RequestBody.Required, "%s requestBody must be required", key)
				concrete := false
				for _, media := range operation.RequestBody.Content {
					concrete = concrete || media.Schema.Ref != ""
				}
				require.True(t, concrete, "%s requestBody needs a concrete schema", key)
			}
			if expected, ok := queryParameters[key]; ok {
				actual := make([]string, 0)
				for _, parameter := range operation.Parameters {
					if parameter.In == "query" {
						actual = append(actual, parameter.Name)
					}
				}
				sort.Strings(actual)
				require.Equal(t, expected, actual, "%s query parameters", key)
			}
			for status, response := range operation.Responses {
				if jsonMedia, ok := response.Content["application/json"]; ok {
					require.NotEmpty(t, jsonMedia.Schema.Ref, "%s %s response needs a concrete schema", key, status)
					require.NotEqual(t, "#/components/schemas/JSONValue", jsonMedia.Schema.Ref)
				}
			}
		}
	}

	backup := decodeOpenAPIOperation(t, contract, http.MethodGet, "/api/v1/backups/{filename}")
	require.Contains(t, backup.Responses["200"].Content, "application/vnd.video-record.backup")
	export := decodeOpenAPIOperation(t, contract, http.MethodGet, "/api/v1/data/export")
	require.Contains(t, export.Responses["200"].Content, "application/json")
	require.Contains(t, export.Responses["200"].Content, "text/csv; charset=utf-8")
}

func TestContractDocumentsTMDBConnectivity(t *testing.T) {
	contract := readOpenAPIContract(t)
	operation := decodeOpenAPIOperation(t, contract, http.MethodGet, "/api/v1/tmdb/connectivity")
	require.Equal(t, "testTMDBConnectivity", operation.OperationID)
	require.Equal(t, "#/components/schemas/TMDBConnectivity", operation.Responses["200"].Content["application/json"].Schema.Ref)

	schema, ok := contract.Components.Schemas["TMDBConnectivity"]
	require.True(t, ok)
	require.Equal(t, []string{"connected"}, schema.Required)
	require.Contains(t, schema.Properties, "connected")
}

func readOpenAPIContract(t *testing.T) openAPIContract {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join("..", "..", "api", "openapi.yaml"))
	require.NoError(t, err)
	var contract openAPIContract
	require.NoError(t, yaml.Unmarshal(contents, &contract))
	return contract
}

func decodeOpenAPIOperation(t *testing.T, contract openAPIContract, method, path string) openAPIOperation {
	t.Helper()
	pathItem, ok := contract.Paths[path]
	require.True(t, ok, "%s is missing", path)
	node, ok := pathItem[strings.ToLower(method)]
	require.True(t, ok, "%s %s is missing", method, path)
	var operation openAPIOperation
	require.NoError(t, node.Decode(&operation))
	return operation
}

func isHTTPMethod(value string) bool {
	switch value {
	case "GET", "POST", "PUT", "DELETE", "PATCH":
		return true
	default:
		return false
	}
}

func newFullContractRouter(t *testing.T) (http.Handler, *storage.DB) {
	t.Helper()
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "video-record.db"))
	require.NoError(t, err)
	require.NoError(t, storage.Migrate(ctx, db))
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	authService := auth.NewService(auth.NewRepository(db), auth.ServiceOptions{})
	_, err = authService.Initialize(ctx, "owner", "correct horse battery staple")
	require.NoError(t, err)
	return NewRouter(Dependencies{
		Storage:   db,
		Auth:      authService,
		TMDB:      tmdb.NewClient(tmdb.ClientOptions{Cache: tmdb.NewCache(db, nil)}),
		Media:     media.NewService(media.NewRepository(db)),
		Records:   records.NewService(records.NewRepository(db)),
		Stats:     statsdomain.NewService(statsdomain.NewRepository(db)),
		Household: household.NewService(household.NewRepository(db)),
		Backup:    storage.NewBackupManager(db, storage.BackupOptions{BackupsDir: filepath.Join(t.TempDir(), "backups")}),
		Sync:      syncdomain.NewCandidateService(db, syncdomain.CandidateServiceOptions{}),
		IntegrationAccounts: integrations.NewAccountRepository(
			db, integrations.NewCredentialCipher(make([]byte, 32)), integrations.AccountRepositoryOptions{},
		),
		SyncJobs: syncdomain.NewService(db, syncdomain.ServiceOptions{}),
	}), db
}
