package support

import (
	"os"
	"strings"
)

func isolatedEnvironment(adminToken string) []string {
	ignored := map[string]struct{}{
		"DB_USER":                      {},
		"DB_PASSWORD":                  {},
		"DB_NAME":                      {},
		"DB_HOST":                      {},
		"DB_PORT":                      {},
		"DB_SSLMODE":                   {},
		"RABBITMQ_URL":                 {},
		"WUZAPI_ADMIN_TOKEN":           {},
		"WUZAPI_GLOBAL_WEBHOOK":        {},
		"WUZAPI_GLOBAL_PROXY_URL":      {},
		"WUZAPI_GLOBAL_HMAC_KEY":       {},
		"WUZAPI_GLOBAL_ENCRYPTION_KEY": {},
	}

	environment := make([]string, 0, len(os.Environ())+4)
	for _, entry := range os.Environ() {
		name, _, found := strings.Cut(entry, "=")
		if !found {
			continue
		}

		if _, shouldIgnore := ignored[name]; shouldIgnore {
			continue
		}

		environment = append(environment, entry)
	}

	return append(environment,
		"WUZAPI_ADMIN_TOKEN="+adminToken,
		"WUZAPI_GLOBAL_ENCRYPTION_KEY=0123456789abcdef0123456789abcdef",
		"WUZAPI_GLOBAL_HMAC_KEY=abcdef0123456789abcdef0123456789",
		"WUZAPI_GLOBAL_WEBHOOK=",
		"WUZAPI_GLOBAL_PROXY_URL=",
		"DB_USER=",
		"DB_PASSWORD=",
		"DB_NAME=",
		"DB_HOST=",
		"DB_PORT=",
		"DB_SSLMODE=",
		"RABBITMQ_URL=",
	)
}
