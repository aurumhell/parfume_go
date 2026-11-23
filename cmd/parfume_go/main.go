package main

import (
	"encoding/csv"
	"errors"
	"html/template"
	"log"
	"net/http"
	"net/mail"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

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

type PageData struct {
	Title       string
	Description string
	Current     string
	IsHome      bool
	Products    []Product
	Advantages  []Advantage
	Navigation  []NavigationItem
	Query       string
	Message     string
	Collection  string
}

type Server struct {
	pages    map[string]*template.Template
	products []Product
	rootDir  string
}

const githubAssetRoot = "https://raw.githubusercontent.com/aurumhell/Odoris/main/assets/"

var navigation = []NavigationItem{
	{Title: "Каталог", URL: "/catalog", Key: "catalog"},
	{Title: "О нас", URL: "/about", Key: "about"},
	{Title: "Доставка", URL: "/delivery", Key: "delivery"},
	{Title: "Новинки", URL: "/new", Key: "new"},
}

var advantages = []Advantage{
	{
		Title: "Бережная доставка",
		Text:  "Отправляем заказы в день покупки. Курьером до двери или в удобный пункт выдачи. Бесплатно — от 5 000 ₽.",
		Icon:  "01",
	},
	{
		Title: "Только оригинал",
		Text:  "Работаем с проверенными поставщиками и внимательно следим за качеством каждого флакона.",
		Icon:  "02",
	},
	{
		Title: "Ароматный комплимент",
		Text:  "Добавляем к заказу миниатюры, чтобы вы могли открыть для себя новые композиции.",
		Icon:  "03",
	},
	{
		Title: "Удобная оплата",
		Text:  "Выбирайте комфортный способ оплаты. Данные передаются по защищённому соединению.",
		Icon:  "04",
	},
}

func main() {
	server, err := newServer()
	if err != nil {
		log.Fatalf("не удалось подготовить сайт: %v", err)
	}

	address := ":8080"
	if value := strings.TrimSpace(os.Getenv("ADDR")); value != "" {
		address = value
	}

	httpServer := &http.Server{
		Addr:              address,
		Handler:           server.routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("Odoris запущен: http://localhost%s", address)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("ошибка HTTP-сервера: %v", err)
	}
}

func newServer() (*Server, error) {
	rootDir, err := findProjectRoot()
	if err != nil {
		return nil, err
	}

	products := []Product{
		{
			ID: "flora-mystique", Name: "Flora Mystique", Brand: "ODORIS COLLECTION",
			Description: "Цветочный · пудровый", Price: "9 500 ₽", PriceValue: 9500,
			Tag: "Бестселлер", ImageURL: githubAssetRoot + "flora.png",
		},
		{
			ID: "ambre-imperial", Name: "Ambre Impérial", Brand: "ODORIS COLLECTION",
			Description: "Амбровый · тёплый", Price: "11 800 ₽", PriceValue: 11800,
			Tag: "Выбор редакции", ImageURL: githubAssetRoot + "ambre.png", IsNew: true,
		},
		{
			ID: "oceanic-drift", Name: "Oceanic Drift", Brand: "ODORIS COLLECTION",
			Description: "Морской · акватический", Price: "8 900 ₽", PriceValue: 8900,
			Tag: "Лёгкий характер", ImageURL: githubAssetRoot + "ocean.png",
		},
		{
			ID: "rose-de-damas", Name: "Rose de Damas", Brand: "ODORIS COLLECTION",
			Description: "Роза · бархатный", Price: "13 400 ₽", PriceValue: 13400,
			Tag: "Изысканный", ImageURL: githubAssetRoot + "roses_parf.png", IsNew: true,
		},
		{
			ID: "velvet-noir", Name: "Velvet Noir", Brand: "ODORIS COLLECTION",
			Description: "Древесный · пряный", Price: "14 200 ₽", PriceValue: 14200,
			Tag: "Вечерний аромат", ImageURL: githubAssetRoot + "velvet.png",
		},
		{
			ID: "the-blanc", Name: "Thé Blanc", Brand: "ODORIS COLLECTION",
			Description: "Белый чай · бергамот", Price: "7 600 ₽", PriceValue: 7600,
			Tag: "На каждый день", ImageURL: githubAssetRoot + "tea.png",
		},
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
			return nil, err
		}
		pages[name] = page
	}

	return &Server{pages: pages, products: products, rootDir: rootDir}, nil
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
			return candidate, nil
		}
	}

	return "", errors.New("не найдена папка проекта с web/templates/layout.html; откройте проект из корневой папки")
}

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	data := s.baseData("Главная", true)
	data.Products = s.products
	switch r.URL.Query().Get("subscribed") {
	case "1":
		data.Message = "Спасибо! Вы подписались на новости Odoris."
	case "error":
		data.Message = "Проверьте адрес электронной почты и подтвердите согласие на рассылку."
	}
	s.render(w, "home", data)
}

func (s *Server) catalog(w http.ResponseWriter, _ *http.Request) {
	data := s.baseData("Каталог", false)
	data.Products = s.products
	s.render(w, "catalog", data)
}

func (s *Server) newArrivals(w http.ResponseWriter, _ *http.Request) {
	data := s.baseData("Новинки", false)
	for _, product := range s.products {
		if product.IsNew {
			data.Products = append(data.Products, product)
		}
	}
	s.render(w, "new", data)
}

func (s *Server) about(w http.ResponseWriter, _ *http.Request) {
	data := s.baseData("О нас", false)
	data.Description = "Odoris — пространство оригинальной парфюмерии, в котором собраны выразительные композиции и ароматы с характером. Мы верим, что парфюм — это тонкий способ рассказать о себе без слов, и помогаем найти звучание, которое станет частью вашей истории."
	s.render(w, "content", data)
}

func (s *Server) delivery(w http.ResponseWriter, _ *http.Request) {
	data := s.baseData("Доставка", false)
	data.Description = "Мы бережно упакуем ваш заказ и передадим его в доставку в день покупки. Вы можете выбрать курьера до двери или удобный пункт выдачи. При заказе от 5 000 рублей доставка бесплатная."
	s.render(w, "content", data)
}

func (s *Server) collectionPage(title, collection string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data := s.baseData(title, false)
		data.Collection = collection
		data.Products = s.products
		if collection == "cart" {
			data.Description = "Ваши выбранные ароматы. Список хранится в браузере на этом устройстве."
			if r.URL.Query().Get("order") == "success" {
				data.Message = "Спасибо! Заявка №" + r.URL.Query().Get("number") + " записана. Мы свяжемся с вами для подтверждения деталей."
			}
		} else {
			data.Description = "Композиции, к которым хочется вернуться. Избранное хранится в браузере на этом устройстве."
		}
		s.render(w, "content", data)
	}
}

func (s *Server) profile(w http.ResponseWriter, _ *http.Request) {
	data := s.baseData("Личный кабинет", false)
	data.Description = "Сохраните контактные данные на этом устройстве, чтобы быстрее заполнять формы при следующих посещениях. Это демонстрационный локальный профиль без регистрации и серверной авторизации."
	s.render(w, "content", data)
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	data := s.baseData("Поиск", false)
	data.Query = query
	if query != "" {
		needle := strings.ToLower(query)
		for _, product := range s.products {
			searchable := strings.ToLower(product.Name + " " + product.Brand + " " + product.Description + " " + product.Tag)
			if strings.Contains(searchable, needle) {
				data.Products = append(data.Products, product)
			}
		}
	}
	s.render(w, "search", data)
}

func (s *Server) subscribe(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || r.FormValue("consent") != "yes" {
		http.Redirect(w, r, "/?subscribe=error", http.StatusSeeOther)
		return
	}

	email := strings.TrimSpace(r.FormValue("email"))
	parsed, err := mail.ParseAddress(email)
	if err != nil || !strings.EqualFold(parsed.Address, email) {
		http.Redirect(w, r, "/?subscribe=error", http.StatusSeeOther)
		return
	}

	if err := s.saveSubscriber(email); err != nil {
		log.Printf("не удалось сохранить подписку: %v", err)
		http.Error(w, "Не удалось сохранить подписку. Попробуйте позже.", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/?subscribed=1", http.StatusSeeOther)
}

func (s *Server) saveSubscriber(email string) error {
	dataDir := filepath.Join(s.rootDir, "data")
	if err := os.MkdirAll(dataDir, 0750); err != nil {
		return err
	}

	path := filepath.Join(dataDir, "subscribers.csv")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}
	writer := csv.NewWriter(file)
	if info.Size() == 0 {
		if err := writer.Write([]string{"email", "subscribed_at"}); err != nil {
			return err
		}
	}
	if err := writer.Write([]string{email, time.Now().Format(time.RFC3339)}); err != nil {
		return err
	}
	writer.Flush()
	return writer.Error()
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
	requested := make(map[string]bool, len(requestedIDs))
	selected := make([]Product, 0, len(requestedIDs))
	for _, rawID := range requestedIDs {
		id := strings.TrimSpace(rawID)
		if id == "" || requested[id] {
			continue
		}
		var found *Product
		for i := range s.products {
			if s.products[i].ID == id {
				found = &s.products[i]
				break
			}
		}
		if found == nil {
			http.Error(w, "В корзине обнаружен неизвестный товар. Обновите страницу и попробуйте снова.", http.StatusBadRequest)
			return
		}
		requested[id] = true
		selected = append(selected, *found)
	}
	if len(selected) == 0 {
		http.Error(w, "Корзина пуста. Добавьте хотя бы один аромат.", http.StatusBadRequest)
		return
	}

	orderNumber := strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
	productNames := make([]string, 0, len(selected))
	total := 0
	for _, product := range selected {
		productNames = append(productNames, product.Name)
		total += product.PriceValue
	}
	if err := s.saveOrder(orderNumber, name, phone, email, address, delivery, strings.Join(productNames, ", "), total); err != nil {
		log.Printf("не удалось сохранить заказ %s: %v", orderNumber, err)
		http.Error(w, "Не удалось сохранить заказ. Попробуйте позже.", http.StatusInternalServerError)
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

func (s *Server) saveOrder(number, name, phone, email, address, delivery, products string, total int) error {
	dataDir := filepath.Join(s.rootDir, "data")
	if err := os.MkdirAll(dataDir, 0750); err != nil {
		return err
	}
	path := filepath.Join(dataDir, "orders.csv")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}
	writer := csv.NewWriter(file)
	if info.Size() == 0 {
		if err := writer.Write([]string{"order_number", "created_at", "name", "phone", "email", "delivery", "address", "products", "total_rub"}); err != nil {
			return err
		}
	}
	if err := writer.Write([]string{number, time.Now().Format(time.RFC3339), name, phone, email, delivery, address, products, strconv.Itoa(total)}); err != nil {
		return err
	}
	writer.Flush()
	return writer.Error()
}

func (s *Server) baseData(title string, isHome bool) PageData {
	return PageData{
		Title:      title,
		Current:    title,
		IsHome:     isHome,
		Navigation: navigation,
		Advantages: advantages,
	}
}

func (s *Server) render(w http.ResponseWriter, page string, data PageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.pages[page].ExecuteTemplate(w, "layout", data); err != nil {
		log.Printf("ошибка рендеринга страницы %q: %v", page, err)
		http.Error(w, "Внутренняя ошибка сервера", http.StatusInternalServerError)
	}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' https://raw.githubusercontent.com data:; style-src 'self' https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com; script-src 'self'; form-action 'self'; base-uri 'self'; frame-ancestors 'self'")
		next.ServeHTTP(w, r)
	})
}
