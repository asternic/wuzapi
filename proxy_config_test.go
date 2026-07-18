package main

import "testing"

func TestResolveWebhookUseProxy(t *testing.T) {
	original := *globalWebhookUseProxy
	defer func() { *globalWebhookUseProxy = original }()

	*globalWebhookUseProxy = true
	if got := resolveWebhookUseProxy(nil); got != true {
		t.Fatalf("expected global default true, got %v", got)
	}

	useProxy := false
	if got := resolveWebhookUseProxy(&useProxy); got != false {
		t.Fatalf("expected per-user override false, got %v", got)
	}

	*globalWebhookUseProxy = false
	if got := resolveWebhookUseProxy(nil); got != false {
		t.Fatalf("expected global default false, got %v", got)
	}

	useProxy = true
	if got := resolveWebhookUseProxy(&useProxy); got != true {
		t.Fatalf("expected per-user override true, got %v", got)
	}
}

func TestProxyConfigResponse(t *testing.T) {
	response := proxyConfigResponse("socks5://127.0.0.1:1080", false)

	if response["enabled"] != true {
		t.Fatalf("expected enabled true when proxy URL is set, got %v", response["enabled"])
	}
	if response["proxy_url"] != "socks5://127.0.0.1:1080" {
		t.Fatalf("unexpected proxy_url: %v", response["proxy_url"])
	}
	if response["webhook_use_proxy"] != false {
		t.Fatalf("expected webhook_use_proxy false, got %v", response["webhook_use_proxy"])
	}
}