package main

import (
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	root, err := findProjectRoot()
	if err != nil {
		t.Fatalf("findProjectRoot() error = %v", err)
	}
	server, err := newServerWithDB(root, filepath.Join(t.TempDir(), "odoris-test.db"))
	if err != nil {
		t.Fatalf("newServerWithDB() error = %v", err)
	}
	t.Cleanup(func() { _ = server.db.Close() })
	return server
}

func TestHomeRendersDatabaseContent(t *testing.T) {
	server := newTestServer(t)
	recorder := httptest.NewRecorder()
	server.home(recorder, httptest.NewRequest("GET", "/", nil))
	if recorder.Code != 200 {
		t.Fatalf("home status = %d, want 200; body: %s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, expected := range []string{"Ароматы,", "Композиции с характером", "Внимание — в каждой детали", "Истории наших клиентов", "Письма с ароматом вдохновения", "site-menu", "flora.png", "data-add-cart", "/catalog"} {
		if !strings.Contains(body, expected) {
			t.Errorf("home page does not contain %q", expected)
		}
	}
}

func TestSearchQueriesDatabase(t *testing.T) {
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

func TestAllMainRoutes(t *testing.T) {
	server := newTestServer(t)
	handler := server.routes()
	for _, path := range []string{"/catalog", "/about", "/delivery", "/new", "/favorites", "/cart", "/profile", "/healthz"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest("GET", path, nil))
		if recorder.Code != 200 {
			t.Errorf("GET %s status = %d, want 200; body=%s", path, recorder.Code, recorder.Body.String())
		}
		if path == "/cart" && !strings.Contains(recorder.Body.String(), `data-collection-page="cart"`) {
			t.Error("cart page is missing its collection container")
		}
		if path == "/cart" && !strings.Contains(recorder.Body.String(), `id="checkout-form"`) {
			t.Error("cart page is missing the checkout form")
		}
		if path == "/profile" && !strings.Contains(recorder.Body.String(), `id="profile-form"`) {
			t.Error("profile page is missing its profile form")
		}
	}
}

func TestSubscribeRequiresConsentAndStoresEmail(t *testing.T) {
	server := newTestServer(t)
	withoutConsent := httptest.NewRecorder()
	server.subscribe(withoutConsent, httptest.NewRequest("POST", "/subscribe", strings.NewReader("email=hello%40example.com")))
	if withoutConsent.Code != 303 {
		t.Fatalf("status without consent = %d, want 303", withoutConsent.Code)
	}

	invalidBody := url.Values{"email": {"not-an-email"}, "consent": {"yes"}}
	invalidRequest := httptest.NewRequest("POST", "/subscribe", strings.NewReader(invalidBody.Encode()))
	invalidRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	invalid := httptest.NewRecorder()
	server.subscribe(invalid, invalidRequest)
	if invalid.Code != 303 {
		t.Fatalf("status for invalid email = %d, want 303", invalid.Code)
	}

	acceptedBody := url.Values{"email": {"hello@example.com"}, "consent": {"yes"}}
	acceptedRequest := httptest.NewRequest("POST", "/subscribe", strings.NewReader(acceptedBody.Encode()))
	acceptedRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	accepted := httptest.NewRecorder()
	server.subscribe(accepted, acceptedRequest)
	if accepted.Code != 303 {
		t.Fatalf("status with consent = %d, want 303; body=%s", accepted.Code, accepted.Body.String())
	}
	var count int
	if err := server.db.QueryRow(`SELECT count(*) FROM subscribers WHERE email = ?`, "hello@example.com").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("subscriber count = %d, want 1", count)
	}
}

func TestCheckoutStoresOrderInDatabase(t *testing.T) {
	server := newTestServer(t)
	body := url.Values{
		"name": {"Test Customer"}, "phone": {"+7 900 123-45-67"}, "email": {"test@example.com"},
		"delivery": {"courier"}, "address": {"Moscow, Main Street 1"}, "products": {"flora-mystique,the-blanc"},
	}
	request := httptest.NewRequest("POST", "/checkout", strings.NewReader(body.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	server.checkout(recorder, request)
	if recorder.Code != 303 {
		t.Fatalf("valid checkout status = %d, want 303; body: %s", recorder.Code, recorder.Body.String())
	}
	location := recorder.Header().Get("Location")
	if !strings.HasPrefix(location, "/cart?order=success&number=") {
		t.Fatalf("checkout redirect = %q, want order confirmation", location)
	}
	orderNumber := strings.TrimPrefix(location, "/cart?order=success&number=")
	var total, itemCount int
	if err := server.db.QueryRow(`SELECT total_rub FROM orders WHERE order_number = ?`, orderNumber).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if err := server.db.QueryRow(`SELECT count(*) FROM order_items WHERE order_number = ?`, orderNumber).Scan(&itemCount); err != nil {
		t.Fatal(err)
	}
	if total != 17100 {
		t.Fatalf("order total = %d, want 17100", total)
	}
	if itemCount != 2 {
		t.Fatalf("order item count = %d, want 2", itemCount)
	}
}

func TestBrowserStateIsStoredInDatabase(t *testing.T) {
	server := newTestServer(t)
	first := httptest.NewRecorder()
	server.apiState(first, httptest.NewRequest("GET", "/api/state", nil))
	if first.Code != 200 {
		t.Fatalf("GET state status = %d", first.Code)
	}
	cookie := first.Result().Cookies()[0]
	payload := `{"cart":["flora-mystique"],"favorites":["the-blanc"],"profile":{"name":"Test User","email":"test@example.com"}}`
	request := httptest.NewRequest("POST", "/api/state", strings.NewReader(payload))
	request.AddCookie(cookie)
	request.Header.Set("Content-Type", "application/json")
	posted := httptest.NewRecorder()
	server.apiState(posted, request)
	if posted.Code != 200 {
		t.Fatalf("POST state status = %d; body=%s", posted.Code, posted.Body.String())
	}
	var cartCount, favoriteCount, profileCount int
	if err := server.db.QueryRow(`SELECT count(*) FROM cart_items WHERE product_id = 'flora-mystique'`).Scan(&cartCount); err != nil {
		t.Fatal(err)
	}
	if err := server.db.QueryRow(`SELECT count(*) FROM favorite_items WHERE product_id = 'the-blanc'`).Scan(&favoriteCount); err != nil {
		t.Fatal(err)
	}
	if err := server.db.QueryRow(`SELECT count(*) FROM profiles WHERE display_name = 'Test User'`).Scan(&profileCount); err != nil {
		t.Fatal(err)
	}
	if cartCount != 1 || favoriteCount != 1 || profileCount != 1 {
		t.Fatalf("database user state counts = cart:%d favorites:%d profiles:%d", cartCount, favoriteCount, profileCount)
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

func TestDatabaseFileCreated(t *testing.T) {
	root, err := findProjectRoot()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "subdir", "odoris.db")
	server, err := newServerWithDB(root, path)
	if err != nil {
		t.Fatal(err)
	}
	defer server.db.Close()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("database file was not created: %v", err)
	}
}
