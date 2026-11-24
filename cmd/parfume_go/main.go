package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	_ "embed"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/mail"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var databaseSchema string

type Product struct {
	ID          string
	Name        string
	Brand       string
	Description string
	Price       string
	PriceValue  int
	Tag         string
	ImageURL    string
	IsNew       bool
}

type Advantage struct {
	Title string
	Text  string
	Icon  string
}

type NavigationItem struct {
	Title string
	URL   string
	Key   string
}

type Review struct {
	Author    string
	City      string
	Body      string
	Stars     int
	StarsText string
}

type PagePoint struct {
	Number string
	Title  string
	Text   string
}

type BrowserState struct {
	Cart      []string `json:"cart"`
	Favorites []string `json:"favorites"`
	Profile   struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"profile"`
}

type PageData struct {
	Title        string
	Description  string
	Current      string
	IsHome       bool
	Products     []Product
	Advantages   []Advantage
	Navigation   []NavigationItem
	Reviews      []Review
	PagePoints   []PagePoint
	Settings     map[string]string
	Query        string
	Message      string
	Collection   string
	ProfileName  string
	ProfileEmail string
}

type Server struct {
	pages   map[string]*template.Template
	db      *sql.DB
	rootDir string
}

func main() {
	server, err := newServer()
	if err != nil {
		log.Fatalf("не удалось подготовить сайт: %v", err)
	}
	defer server.db.Close()

	address := ":8080"
	if value := strings.TrimSpace(os.Getenv("ADDR")); value != "" {
		address = value
	}
	httpServer := &http.Server{
		Addr:              address,
		Handler:           server.routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Printf("Odoris запущен: http://localhost%s", address)
	log.Printf("SQLite: %s", server.databasePath())
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("ошибка HTTP-сервера: %v", err)
	}
}

func newServer() (*Server, error) {
	rootDir, err := findProjectRoot()
	if err != nil {
		return nil, err
	}
	databasePath := strings.TrimSpace(os.Getenv("DB_PATH"))
	if databasePath == "" {
		databasePath = filepath.Join(rootDir, "data", "odoris.db")
	} else if !filepath.IsAbs(databasePath) {
		databasePath = filepath.Join(rootDir, databasePath)
	}
	return newServerWithDB(rootDir, databasePath)
}

func newServerWithDB(rootDir, databasePath string) (*Server, error) {
	if err := os.MkdirAll(filepath.Dir(databasePath), 0750); err != nil {
		return nil, fmt.Errorf("создать папку базы данных: %w", err)
	}
	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		return nil, fmt.Errorf("открыть SQLite: %w", err)
	}
	// A single connection keeps SQLite pragmas and writes predictable for this small site.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("подключиться к SQLite: %w", err)
	}
	for _, statement := range []string{
		`PRAGMA foreign_keys = ON`,
		`PRAGMA journal_mode = WAL`,
		`PRAGMA busy_timeout = 5000`,
	} {
		if _, err := db.Exec(statement); err != nil {
			db.Close()
			return nil, fmt.Errorf("настроить SQLite (%s): %w", statement, err)
		}
	}
	if err := applyDatabaseSchema(db, databaseSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("создать схему базы данных: %w", err)
	}
	if err := migrateLegacyCSV(rootDir, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("перенести старые CSV-данные в SQLite: %w", err)
	}

	pages := make(map[string]*template.Template)
	for _, name := range []string{"home", "catalog", "content", "new", "search"} {
		page, err := template.New("site").Funcs(template.FuncMap{
			"nowYear": func() int { return time.Now().Year() },
		}).ParseFiles(
			filepath.Join(rootDir, "web/templates/layout.html"),
			filepath.Join(rootDir, "web/templates/products.html"),
			filepath.Join(rootDir, "web/templates", name+".html"),
		)
		if err != nil {
			db.Close()
			return nil, fmt.Errorf("загрузить шаблон %s: %w", name, err)
		}
		pages[name] = page
	}
	return &Server{pages: pages, db: db, rootDir: rootDir}, nil
}

func (s *Server) databasePath() string {
	if value := strings.TrimSpace(os.Getenv("DB_PATH")); value != "" {
		return value
	}
	return filepath.Join(s.rootDir, "data", "odoris.db")
}

func applyDatabaseSchema(db *sql.DB, schema string) error {
	for _, statement := range strings.Split(schema, ";") {
		statement = strings.TrimSpace(statement)
		if statement == "" {
			continue
		}
		if _, err := db.Exec(statement); err != nil {
			return fmt.Errorf("SQL: %s: %w", statement, err)
		}
	}
	return nil
}

func migrateLegacyCSV(rootDir string, db *sql.DB) error {
	dataDir := filepath.Join(rootDir, "data")
	if err := importLegacySubscribers(filepath.Join(dataDir, "subscribers.csv"), db); err != nil {
		return err
	}
	if err := importLegacyOrders(filepath.Join(dataDir, "orders.csv"), db); err != nil {
		return err
	}
	return nil
}

func importLegacySubscribers(path string, db *sql.DB) error {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()
	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	if _, err := reader.Read(); err != nil {
		return err
	} // header
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if len(record) < 1 {
			continue
		}
		email := strings.TrimSpace(record[0])
		parsed, parseErr := mail.ParseAddress(email)
		if parseErr != nil || !strings.EqualFold(parsed.Address, email) {
			continue
		}
		subscribedAt := time.Now().UTC().Format(time.RFC3339)
		if len(record) > 1 && strings.TrimSpace(record[1]) != "" {
			subscribedAt = strings.TrimSpace(record[1])
		}
		if _, err := db.Exec(`INSERT OR IGNORE INTO subscribers(email, subscribed_at, consent_source) VALUES(?, ?, 'legacy_csv')`, email, subscribedAt); err != nil {
			return err
		}
	}
}

func importLegacyOrders(path string, db *sql.DB) error {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()
	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	if _, err := reader.Read(); err != nil {
		return err
	} // header
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if len(record) < 9 {
			continue
		}
		orderNumber := strings.TrimSpace(record[0])
		if orderNumber == "" {
			continue
		}
		total, parseErr := strconv.Atoi(strings.TrimSpace(record[8]))
		if parseErr != nil || total < 0 {
			continue
		}
		createdAt := strings.TrimSpace(record[1])
		if createdAt == "" {
			createdAt = time.Now().UTC().Format(time.RFC3339)
		}
		result, err := db.Exec(`INSERT OR IGNORE INTO orders(order_number, created_at, customer_name, phone, email, delivery_method, address, total_rub) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`, orderNumber, createdAt, record[2], record[3], record[4], record[5], record[6], total)
		if err != nil {
			return err
		}
		inserted, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if inserted == 0 {
			continue
		}
		productNames := strings.Split(record[7], ", ")
		for _, productName := range productNames {
			productName = strings.TrimSpace(productName)
			if productName == "" {
				continue
			}
			var productID sql.NullString
			_ = db.QueryRow(`SELECT id FROM products WHERE name = ? LIMIT 1`, productName).Scan(&productID)
			if _, err := db.Exec(`INSERT INTO order_items(order_number, product_id, product_name, unit_price_rub, quantity) VALUES(?, ?, ?, 0, 1)`, orderNumber, nullableString(productID), productName); err != nil {
				return err
			}
		}
	}
}

func nullableString(value sql.NullString) any {
	if !value.Valid {
		return nil
	}
	return value.String
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", s.staticHandler()))
	mux.HandleFunc("GET /{$}", s.home)
	mux.HandleFunc("GET /catalog", s.catalog)
	mux.HandleFunc("GET /about", s.about)
	mux.HandleFunc("GET /delivery", s.delivery)
	mux.HandleFunc("GET /new", s.newArrivals)
	mux.HandleFunc("GET /favorites", s.collectionPage("Избранное", "favorites"))
	mux.HandleFunc("GET /cart", s.collectionPage("Корзина", "cart"))
	mux.HandleFunc("GET /profile", s.profile)
	mux.HandleFunc("GET /search", s.search)
	mux.HandleFunc("POST /subscribe", s.subscribe)
	mux.HandleFunc("POST /checkout", s.checkout)
	mux.HandleFunc("GET /api/state", s.apiState)
	mux.HandleFunc("POST /api/state", s.apiState)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return securityHeaders(mux)
}

func (s *Server) staticHandler() http.Handler {
	return http.FileServer(http.Dir(filepath.Join(s.rootDir, "web/static")))
}

func findProjectRoot() (string, error) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		return "", err
	}
	candidates := []string{workingDirectory}
	if _, sourceFile, _, ok := runtime.Caller(0); ok {
		packageDirectory := filepath.Dir(sourceFile)
		candidates = append(candidates, filepath.Clean(filepath.Join(packageDirectory, "../..")))
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(filepath.Join(candidate, "web/templates/layout.html")); err == nil {
			if _, err := os.Stat(filepath.Join(candidate, "web/static/main.js")); err == nil {
				return candidate, nil
			}
		}
	}
	return "", errors.New("не найдена папка проекта с web/templates/layout.html; запустите Go из корневой папки проекта")
}

func (s *Server) products(ctx context.Context, mode, query string) ([]Product, error) {
	sqlQuery := `SELECT id, name, brand, description, price_rub, tag, image_url, is_new FROM products WHERE is_active = 1`
	args := make([]any, 0, 1)
	switch mode {
	case "new":
		sqlQuery += ` AND is_new = 1`
	case "search":
		sqlQuery += ` AND lower(name || ' ' || brand || ' ' || description || ' ' || tag) LIKE ?`
		args = append(args, "%"+strings.ToLower(query)+"%")
	}
	sqlQuery += ` ORDER BY rowid`
	rows, err := s.db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	products := make([]Product, 0)
	for rows.Next() {
		var product Product
		var price int
		var isNew int
		if err := rows.Scan(&product.ID, &product.Name, &product.Brand, &product.Description, &price, &product.Tag, &product.ImageURL, &isNew); err != nil {
			return nil, err
		}
		product.PriceValue = price
		product.Price = formatPrice(price) + " ₽"
		product.IsNew = isNew == 1
		products = append(products, product)
	}
	return products, rows.Err()
}

func formatPrice(value int) string {
	digits := strconv.Itoa(value)
	for i := len(digits) - 3; i > 0; i -= 3 {
		digits = digits[:i] + " " + digits[i:]
	}
	return digits
}

func (s *Server) baseData(ctx context.Context, title string, isHome bool) (PageData, error) {
	data := PageData{Title: title, Current: title, IsHome: isHome, Settings: make(map[string]string)}
	rows, err := s.db.QueryContext(ctx, `SELECT title, url, item_key FROM navigation_items WHERE is_active = 1 ORDER BY sort_order`)
	if err != nil {
		return data, err
	}
	for rows.Next() {
		var item NavigationItem
		if err := rows.Scan(&item.Title, &item.URL, &item.Key); err != nil {
			rows.Close()
			return data, err
		}
		data.Navigation = append(data.Navigation, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return data, err
	}
	rows.Close()

	rows, err = s.db.QueryContext(ctx, `SELECT title, body, icon FROM advantages WHERE is_active = 1 ORDER BY sort_order`)
	if err != nil {
		return data, err
	}
	for rows.Next() {
		var item Advantage
		if err := rows.Scan(&item.Title, &item.Text, &item.Icon); err != nil {
			rows.Close()
			return data, err
		}
		data.Advantages = append(data.Advantages, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return data, err
	}
	rows.Close()

	rows, err = s.db.QueryContext(ctx, `SELECT setting_key, setting_value FROM site_settings`)
	if err != nil {
		return data, err
	}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			rows.Close()
			return data, err
		}
		data.Settings[key] = value
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return data, err
	}
	rows.Close()
	return data, nil
}

func (s *Server) pageContent(ctx context.Context, slug string) (string, []PagePoint, error) {
	var description string
	if err := s.db.QueryRowContext(ctx, `SELECT description FROM pages WHERE slug = ? AND is_active = 1`, slug).Scan(&description); err != nil {
		return "", nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT sort_order, title, body FROM page_points WHERE page_slug = ? ORDER BY sort_order`, slug)
	if err != nil {
		return "", nil, err
	}
	defer rows.Close()
	points := make([]PagePoint, 0)
	for rows.Next() {
		var order int
		var point PagePoint
		if err := rows.Scan(&order, &point.Title, &point.Text); err != nil {
			return "", nil, err
		}
		point.Number = fmt.Sprintf("%02d", order)
		points = append(points, point)
	}
	return description, points, rows.Err()
}

func (s *Server) reviews(ctx context.Context) ([]Review, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT author, city, body, stars FROM reviews WHERE is_active = 1 ORDER BY sort_order`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Review, 0)
	for rows.Next() {
		var item Review
		if err := rows.Scan(&item.Author, &item.City, &item.Body, &item.Stars); err != nil {
			return nil, err
		}
		item.StarsText = strings.Repeat("★", item.Stars)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	data, err := s.baseData(r.Context(), "Главная", true)
	if err == nil {
		data.Products, err = s.products(r.Context(), "all", "")
	}
	if err == nil {
		data.Reviews, err = s.reviews(r.Context())
	}
	if err != nil {
		s.internalError(w, "загрузить главную страницу", err)
		return
	}
	switch r.URL.Query().Get("subscribed") {
	case "1":
		data.Message = "Спасибо! Вы подписались на новости Odoris."
	case "error":
		data.Message = "Проверьте адрес электронной почты и подтвердите согласие на рассылку."
	}
	s.render(w, "home", data)
}

func (s *Server) catalog(w http.ResponseWriter, r *http.Request) {
	data, err := s.baseData(r.Context(), "Каталог", false)
	if err == nil {
		data.Products, err = s.products(r.Context(), "all", "")
	}
	if err != nil {
		s.internalError(w, "загрузить каталог", err)
		return
	}
	s.render(w, "catalog", data)
}

func (s *Server) newArrivals(w http.ResponseWriter, r *http.Request) {
	data, err := s.baseData(r.Context(), "Новинки", false)
	if err == nil {
		data.Products, err = s.products(r.Context(), "new", "")
	}
	if err != nil {
		s.internalError(w, "загрузить новинки", err)
		return
	}
	s.render(w, "new", data)
}

func (s *Server) about(w http.ResponseWriter, r *http.Request) {
	s.contentPage(w, r, "about", "О нас")
}

func (s *Server) delivery(w http.ResponseWriter, r *http.Request) {
	s.contentPage(w, r, "delivery", "Доставка")
}

func (s *Server) contentPage(w http.ResponseWriter, r *http.Request, slug, title string) {
	data, err := s.baseData(r.Context(), title, false)
	if err == nil {
		data.Description, data.PagePoints, err = s.pageContent(r.Context(), slug)
	}
	if err != nil {
		s.internalError(w, "загрузить страницу "+slug, err)
		return
	}
	s.render(w, "content", data)
}

func (s *Server) collectionPage(title, collection string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := s.baseData(r.Context(), title, false)
		if err == nil {
			data.Products, err = s.products(r.Context(), "all", "")
		}
		if err != nil {
			s.internalError(w, "загрузить страницу коллекции", err)
			return
		}
		data.Collection = collection
		settingKey := "page.favorites.description"
		if collection == "cart" {
			settingKey = "page.cart.description"
		}
		data.Description = data.Settings[settingKey]
		if collection == "cart" && r.URL.Query().Get("order") == "success" {
			data.Message = "Спасибо! Заявка №" + r.URL.Query().Get("number") + " записана. Мы свяжемся с вами для подтверждения деталей."
		}
		s.render(w, "content", data)
	}
}

func (s *Server) profile(w http.ResponseWriter, r *http.Request) {
	data, err := s.baseData(r.Context(), "Личный кабинет", false)
	if err != nil {
		s.internalError(w, "загрузить профиль", err)
		return
	}
	data.Description = data.Settings["page.profile.description"]
	state, err := s.readBrowserState(w, r)
	if err != nil {
		s.internalError(w, "загрузить пользовательские данные", err)
		return
	}
	data.ProfileName = state.Profile.Name
	data.ProfileEmail = state.Profile.Email
	s.render(w, "content", data)
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	data, err := s.baseData(r.Context(), "Поиск", false)
	if err == nil && query != "" {
		data.Products, err = s.products(r.Context(), "search", query)
	}
	if err != nil {
		s.internalError(w, "выполнить поиск", err)
		return
	}
	data.Query = query
	s.render(w, "search", data)
}

func (s *Server) subscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil || r.FormValue("consent") != "yes" {
		http.Redirect(w, r, "/?subscribe=error", http.StatusSeeOther)
		return
	}
	email := strings.TrimSpace(r.FormValue("email"))
	parsed, err := mail.ParseAddress(email)
	if err != nil || !strings.EqualFold(parsed.Address, email) || len(email) > 254 {
		http.Redirect(w, r, "/?subscribe=error", http.StatusSeeOther)
		return
	}
	_, err = s.db.ExecContext(r.Context(), `INSERT INTO subscribers(email, subscribed_at, consent_source) VALUES(?, ?, 'website') ON CONFLICT(email) DO UPDATE SET subscribed_at = excluded.subscribed_at`, email, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		s.internalError(w, "сохранить подписку", err)
		return
	}
	http.Redirect(w, r, "/?subscribed=1", http.StatusSeeOther)
}

func (s *Server) checkout(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Не удалось прочитать данные заказа.", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	phone := strings.TrimSpace(r.FormValue("phone"))
	email := strings.TrimSpace(r.FormValue("email"))
	address := strings.TrimSpace(r.FormValue("address"))
	delivery := strings.TrimSpace(r.FormValue("delivery"))
	parsedEmail, emailErr := mail.ParseAddress(email)
	if name == "" || len(name) > 120 || !isValidPhone(phone) || emailErr != nil || !strings.EqualFold(parsedEmail.Address, email) || len(address) < 5 || len(address) > 500 || (delivery != "courier" && delivery != "pickup") {
		http.Error(w, "Проверьте имя, телефон, электронную почту и адрес доставки.", http.StatusBadRequest)
		return
	}

	requestedIDs := strings.Split(r.FormValue("products"), ",")
	if strings.TrimSpace(r.FormValue("products")) == "" {
		sessionID, err := s.ensureSession(w, r)
		if err != nil {
			s.internalError(w, "получить корзину", err)
			return
		}
		state, err := s.readStateByID(r.Context(), sessionID)
		if err != nil {
			s.internalError(w, "получить корзину", err)
			return
		}
		requestedIDs = state.Cart
	}
	requested := make(map[string]bool, len(requestedIDs))
	selected := make([]Product, 0, len(requestedIDs))
	for _, rawID := range requestedIDs {
		id := strings.TrimSpace(rawID)
		if id == "" || requested[id] {
			continue
		}
		var product Product
		var price, isNew int
		err := s.db.QueryRowContext(r.Context(), `SELECT id, name, brand, description, price_rub, tag, image_url, is_new FROM products WHERE id = ? AND is_active = 1`, id).
			Scan(&product.ID, &product.Name, &product.Brand, &product.Description, &price, &product.Tag, &product.ImageURL, &isNew)
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "В корзине обнаружен неизвестный товар. Обновите страницу и попробуйте снова.", http.StatusBadRequest)
			return
		}
		if err != nil {
			s.internalError(w, "проверить товар в заказе", err)
			return
		}
		product.PriceValue, product.Price, product.IsNew = price, formatPrice(price)+" ₽", isNew == 1
		requested[id] = true
		selected = append(selected, product)
	}
	if len(selected) == 0 {
		http.Error(w, "Корзина пуста. Добавьте хотя бы один аромат.", http.StatusBadRequest)
		return
	}

	orderNumber := strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
	createdAt := time.Now().UTC().Format(time.RFC3339)
	total := 0
	for _, product := range selected {
		total += product.PriceValue
	}
	sessionID, err := s.ensureSession(w, r)
	if err != nil {
		s.internalError(w, "получить пользовательскую сессию", err)
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		s.internalError(w, "начать сохранение заказа", err)
		return
	}
	_, err = tx.ExecContext(r.Context(), `INSERT INTO orders(order_number, created_at, customer_name, phone, email, delivery_method, address, total_rub) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`, orderNumber, createdAt, name, phone, email, delivery, address, total)
	if err == nil {
		for _, product := range selected {
			if _, err = tx.ExecContext(r.Context(), `INSERT INTO order_items(order_number, product_id, product_name, unit_price_rub, quantity) VALUES(?, ?, ?, ?, 1)`, orderNumber, product.ID, product.Name, product.PriceValue); err != nil {
				break
			}
		}
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `DELETE FROM cart_items WHERE session_id = ?`, sessionID)
	}
	if err != nil {
		_ = tx.Rollback()
		s.internalError(w, "сохранить заказ", err)
		return
	}
	if err := tx.Commit(); err != nil {
		s.internalError(w, "завершить сохранение заказа", err)
		return
	}
	http.Redirect(w, r, "/cart?order=success&number="+orderNumber, http.StatusSeeOther)
}

func isValidPhone(value string) bool {
	digits := 0
	for _, char := range value {
		if char >= '0' && char <= '9' {
			digits++
		}
	}
	return digits >= 7 && len(value) <= 40
}

func (s *Server) apiState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}
	sessionID, err := s.ensureSession(w, r)
	if err != nil {
		s.internalError(w, "создать пользовательскую сессию", err)
		return
	}
	if r.Method == http.MethodGet {
		state, err := s.readStateByID(r.Context(), sessionID)
		if err != nil {
			s.internalError(w, "прочитать пользовательскую сессию", err)
			return
		}
		writeJSON(w, http.StatusOK, state)
		return
	}
	var state BrowserState
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := decoder.Decode(&state); err != nil {
		http.Error(w, "Некорректные данные.", http.StatusBadRequest)
		return
	}
	if len(state.Cart) > 100 || len(state.Favorites) > 100 {
		http.Error(w, "Слишком много товаров в списке.", http.StatusBadRequest)
		return
	}
	state.Cart = uniqueStrings(state.Cart)
	state.Favorites = uniqueStrings(state.Favorites)
	state.Profile.Name = strings.TrimSpace(state.Profile.Name)
	state.Profile.Email = strings.TrimSpace(state.Profile.Email)
	if len(state.Profile.Name) > 120 || len(state.Profile.Email) > 254 {
		http.Error(w, "Слишком длинные контактные данные.", http.StatusBadRequest)
		return
	}
	if state.Profile.Email != "" {
		parsed, err := mail.ParseAddress(state.Profile.Email)
		if err != nil || !strings.EqualFold(parsed.Address, state.Profile.Email) {
			http.Error(w, "Некорректный адрес электронной почты.", http.StatusBadRequest)
			return
		}
	}
	if err := s.writeState(r.Context(), sessionID, state); err != nil {
		s.internalError(w, "сохранить пользовательские данные", err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len(value) > 100 || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func (s *Server) ensureSession(w http.ResponseWriter, r *http.Request) (string, error) {
	if cookie, err := r.Cookie("odoris_session"); err == nil && len(cookie.Value) == 64 {
		var exists int
		err := s.db.QueryRowContext(r.Context(), `SELECT count(*) FROM browser_sessions WHERE session_id = ? AND expires_at > ?`, cookie.Value, time.Now().UTC().Format(time.RFC3339)).Scan(&exists)
		if err == nil && exists == 1 {
			_, _ = s.db.ExecContext(r.Context(), `UPDATE browser_sessions SET updated_at = ?, expires_at = ? WHERE session_id = ?`, time.Now().UTC().Format(time.RFC3339), time.Now().UTC().Add(30*24*time.Hour).Format(time.RFC3339), cookie.Value)
			return cookie.Value, nil
		}
	}
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	id := hex.EncodeToString(bytes)
	now := time.Now().UTC()
	_, err := s.db.ExecContext(r.Context(), `INSERT INTO browser_sessions(session_id, created_at, updated_at, expires_at) VALUES(?, ?, ?, ?)`, id, now.Format(time.RFC3339), now.Format(time.RFC3339), now.Add(30*24*time.Hour).Format(time.RFC3339))
	if err != nil {
		return "", err
	}
	http.SetCookie(w, &http.Cookie{
		Name: "odoris_session", Value: id, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, Secure: r.TLS != nil, MaxAge: 30 * 24 * 60 * 60,
	})
	return id, nil
}

func (s *Server) readBrowserState(w http.ResponseWriter, r *http.Request) (BrowserState, error) {
	id, err := s.ensureSession(w, r)
	if err != nil {
		return BrowserState{}, err
	}
	return s.readStateByID(r.Context(), id)
}

func (s *Server) readStateByID(ctx context.Context, sessionID string) (BrowserState, error) {
	state := BrowserState{Cart: []string{}, Favorites: []string{}}
	rows, err := s.db.QueryContext(ctx, `SELECT product_id FROM cart_items WHERE session_id = ? ORDER BY rowid`, sessionID)
	if err != nil {
		return state, err
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return state, err
		}
		state.Cart = append(state.Cart, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return state, err
	}
	rows.Close()
	rows, err = s.db.QueryContext(ctx, `SELECT product_id FROM favorite_items WHERE session_id = ? ORDER BY created_at`, sessionID)
	if err != nil {
		return state, err
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return state, err
		}
		state.Favorites = append(state.Favorites, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return state, err
	}
	rows.Close()
	_ = s.db.QueryRowContext(ctx, `SELECT display_name, email FROM profiles WHERE session_id = ?`, sessionID).Scan(&state.Profile.Name, &state.Profile.Email)
	return state, nil
}

func (s *Server) writeState(ctx context.Context, sessionID string, state BrowserState) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM cart_items WHERE session_id = ?`, sessionID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM favorite_items WHERE session_id = ?`, sessionID); err != nil {
		return err
	}
	for _, id := range state.Cart {
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM products WHERE id = ? AND is_active = 1`, id).Scan(&exists); err != nil {
			return err
		}
		if exists == 1 {
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO cart_items(session_id, product_id, quantity) VALUES(?, ?, 1)`, sessionID, id); err != nil {
				return err
			}
		}
	}
	for _, id := range state.Favorites {
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM products WHERE id = ? AND is_active = 1`, id).Scan(&exists); err != nil {
			return err
		}
		if exists == 1 {
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO favorite_items(session_id, product_id, created_at) VALUES(?, ?, ?)`, sessionID, id, time.Now().UTC().Format(time.RFC3339)); err != nil {
				return err
			}
		}
	}
	if state.Profile.Name == "" && state.Profile.Email == "" {
		if _, err := tx.ExecContext(ctx, `DELETE FROM profiles WHERE session_id = ?`, sessionID); err != nil {
			return err
		}
	} else {
		if _, err := tx.ExecContext(ctx, `INSERT INTO profiles(session_id, display_name, email, updated_at) VALUES(?, ?, ?, ?) ON CONFLICT(session_id) DO UPDATE SET display_name = excluded.display_name, email = excluded.email, updated_at = excluded.updated_at`, sessionID, state.Profile.Name, state.Profile.Email, time.Now().UTC().Format(time.RFC3339)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (s *Server) internalError(w http.ResponseWriter, action string, err error) {
	log.Printf("не удалось %s: %v", action, err)
	http.Error(w, "Внутренняя ошибка сервера", http.StatusInternalServerError)
}

func (s *Server) render(w http.ResponseWriter, page string, data PageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.pages[page].ExecuteTemplate(w, "layout", data); err != nil {
		s.internalError(w, "отрендерить страницу "+page, err)
	}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' https://raw.githubusercontent.com data:; style-src 'self' https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com; script-src 'self'; connect-src 'self'; form-action 'self'; base-uri 'self'; frame-ancestors 'self'")
		next.ServeHTTP(w, r)
	})
}
