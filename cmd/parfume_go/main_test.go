package main

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	server, err := newServer()
	if err != nil {
		t.Fatalf("newServer() error = %v", err)
	}
	return server
}

func TestHomeRendersMainSections(t *testing.T) {
	server := newTestServer(t)
	recorder := httptest.NewRecorder()
	server.home(recorder, httptest.NewRequest("GET", "/", nil))

	if recorder.Code != 200 {
		t.Fatalf("home status = %d, want 200", recorder.Code)
	}
	body := recorder.Body.String()
	for _, expected := range []string{
		"Ароматы,", "Композиции с характером", "Внимание — в каждой детали",
		"Истории наших клиентов", "Письма с ароматом вдохновения", "site-menu",
		"flora.png", "data-add-cart", "/catalog",
	} {
		if !strings.Contains(body, expected) {
			t.Errorf("home page does not contain %q", expected)
		}
	}
}

func TestSearchFiltersProducts(t *testing.T) {
	server := newTestServer(t)
	recorder := httptest.NewRecorder()
	server.search(recorder, httptest.NewRequest("GET", "/search?q=ambre", nil))
	body := recorder.Body.String()

	if recorder.Code != 200 {
		t.Fatalf("search status = %d, want 200", recorder.Code)
	}
	if !strings.Contains(body, "Ambre Impérial") {
		t.Fatal("search result does not contain the matching perfume")
	}
	if strings.Contains(body, "Oceanic Drift") {
		t.Fatal("search result contains a perfume that does not match the query")
	}
}

func TestCollectionAndContentRoutes(t *testing.T) {
	server := newTestServer(t)
	handler := server.routes()
	for _, path := range []string{"/catalog", "/about", "/delivery", "/new", "/favorites", "/cart", "/profile", "/healthz"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest("GET", path, nil))
		if recorder.Code != 200 {
			t.Errorf("GET %s status = %d, want 200", path, recorder.Code)
		}
		if path == "/cart" && !strings.Contains(recorder.Body.String(), `data-collection-page="cart"`) {
			t.Error("cart page is missing its collection container")
		}
		if path == "/cart" && !strings.Contains(recorder.Body.String(), `id="checkout-form"`) {
			t.Error("cart page is missing the checkout form")
		}
		if path == "/profile" && !strings.Contains(recorder.Body.String(), `id="profile-form"`) {
			t.Error("profile page is missing its local profile form")
		}
	}
}

func TestSubscribeRequiresConsentAndStoresEmail(t *testing.T) {
	server := newTestServer(t)
	server.rootDir = t.TempDir()

	withoutConsent := httptest.NewRecorder()
	server.subscribe(withoutConsent, httptest.NewRequest("POST", "/subscribe", strings.NewReader("email=hello%40example.com")))
	if withoutConsent.Code != 303 {
		t.Fatalf("status without consent = %d, want 303", withoutConsent.Code)
	}
	if got := withoutConsent.Header().Get("Location"); got != "/?subscribe=error" {
		t.Fatalf("invalid redirect = %q, want /?subscribe=error", got)
	}

	invalidEmail := httptest.NewRequest("POST", "/subscribe", strings.NewReader("email=not-an-email&consent=yes"))
	invalidEmail.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	invalid := httptest.NewRecorder()
	server.subscribe(invalid, invalidEmail)
	if invalid.Code != 303 {
		t.Fatalf("status for invalid email = %d, want 303", invalid.Code)
	}

	acceptedRequest := httptest.NewRequest("POST", "/subscribe", strings.NewReader("email=hello%40example.com&consent=yes"))
	acceptedRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	accepted := httptest.NewRecorder()
	server.subscribe(accepted, acceptedRequest)
	if accepted.Code != 303 {
		t.Fatalf("status with consent = %d, want 303", accepted.Code)
	}
	if got := accepted.Header().Get("Location"); got != "/?subscribed=1" {
		t.Fatalf("redirect location = %q, want /?subscribed=1", got)
	}

	contents, err := os.ReadFile(filepath.Join(server.rootDir, "data", "subscribers.csv"))
	if err != nil {
		t.Fatalf("subscriber file was not created: %v", err)
	}
	if !strings.Contains(string(contents), "hello@example.com") {
		t.Fatal("subscriber email was not written to CSV")
	}
}

func TestCheckoutStoresOrderAndRejectsUnknownProducts(t *testing.T) {
	server := newTestServer(t)
	server.rootDir = t.TempDir()

	invalidRequest := httptest.NewRequest("POST", "/checkout", strings.NewReader("name=Test&phone=123456789&email=test%40example.com&delivery=courier&address=Somewhere&products=not-a-product"))
	invalidRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	invalid := httptest.NewRecorder()
	server.checkout(invalid, invalidRequest)
	if invalid.Code != 400 {
		t.Fatalf("unknown product status = %d, want 400", invalid.Code)
	}

	request := httptest.NewRequest("POST", "/checkout", strings.NewReader("name=Test+Customer&phone=%2B7+900+123-45-67&email=test%40example.com&delivery=courier&address=Moscow%2C+Main+Street+1&products=flora-mystique%2Cthe-blanc"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	server.checkout(recorder, request)
	if recorder.Code != 303 {
		t.Fatalf("valid checkout status = %d, want 303; body: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.HasPrefix(recorder.Header().Get("Location"), "/cart?order=success&number=") {
		t.Fatalf("checkout redirect = %q, want order confirmation", recorder.Header().Get("Location"))
	}
	contents, err := os.ReadFile(filepath.Join(server.rootDir, "data", "orders.csv"))
	if err != nil {
		t.Fatalf("order file was not created: %v", err)
	}
	for _, expected := range []string{"Test Customer", "test@example.com", "courier", "Flora Mystique", "Thé Blanc", "17100"} {
		if !strings.Contains(string(contents), expected) {
			t.Errorf("saved order does not contain %q", expected)
		}
	}
}

func TestStaticFilesAreServed(t *testing.T) {
	server := newTestServer(t)
	for _, path := range []string{"/main.css", "/main.js"} {
		recorder := httptest.NewRecorder()
		server.staticHandler().ServeHTTP(recorder, httptest.NewRequest("GET", path, nil))
		if recorder.Code != 200 {
			t.Fatalf("static file %s status = %d, want 200", path, recorder.Code)
		}
		if recorder.Body.Len() == 0 {
			t.Errorf("static file %s is empty", path)
		}
	}
}
