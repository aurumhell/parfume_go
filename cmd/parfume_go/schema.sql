PRAGMA foreign_keys = ON;

-- Main catalog. Prices are stored as integer roubles, never as formatted strings.
CREATE TABLE IF NOT EXISTS products (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    brand           TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    price_rub       INTEGER NOT NULL CHECK (price_rub >= 0),
    tag             TEXT NOT NULL DEFAULT '',
    image_url       TEXT NOT NULL DEFAULT '',
    is_new          INTEGER NOT NULL DEFAULT 0 CHECK (is_new IN (0, 1)),
    is_active       INTEGER NOT NULL DEFAULT 1 CHECK (is_active IN (0, 1)),
    created_at      TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_products_active_name ON products(is_active, name);
CREATE INDEX IF NOT EXISTS idx_products_new ON products(is_active, is_new);

-- Menu and homepage content blocks.
CREATE TABLE IF NOT EXISTS navigation_items (
    item_key        TEXT PRIMARY KEY,
    title           TEXT NOT NULL,
    url             TEXT NOT NULL,
    sort_order      INTEGER NOT NULL DEFAULT 0,
    is_active       INTEGER NOT NULL DEFAULT 1 CHECK (is_active IN (0, 1))
);

CREATE TABLE IF NOT EXISTS advantages (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    title           TEXT NOT NULL,
    body            TEXT NOT NULL,
    icon            TEXT NOT NULL DEFAULT '',
    sort_order      INTEGER NOT NULL DEFAULT 0,
    is_active       INTEGER NOT NULL DEFAULT 1 CHECK (is_active IN (0, 1))
);

CREATE TABLE IF NOT EXISTS reviews (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    author          TEXT NOT NULL,
    city            TEXT NOT NULL DEFAULT '',
    body            TEXT NOT NULL,
    stars           INTEGER NOT NULL DEFAULT 5 CHECK (stars BETWEEN 1 AND 5),
    sort_order      INTEGER NOT NULL DEFAULT 0,
    is_active       INTEGER NOT NULL DEFAULT 1 CHECK (is_active IN (0, 1))
);

-- Content pages and their repeatable information cards.
CREATE TABLE IF NOT EXISTS pages (
    slug            TEXT PRIMARY KEY,
    title           TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    is_active       INTEGER NOT NULL DEFAULT 1 CHECK (is_active IN (0, 1))
);

CREATE TABLE IF NOT EXISTS page_points (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    page_slug       TEXT NOT NULL REFERENCES pages(slug) ON DELETE CASCADE,
    title           TEXT NOT NULL,
    body            TEXT NOT NULL,
    sort_order      INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_page_points_page_order ON page_points(page_slug, sort_order);

-- Key/value content lets the Go templates render editable headings and copy from the DB.
CREATE TABLE IF NOT EXISTS site_settings (
    setting_key     TEXT PRIMARY KEY,
    setting_value  TEXT NOT NULL
);

-- Orders preserve a snapshot of the product name and unit price at purchase time.
CREATE TABLE IF NOT EXISTS orders (
    order_number    TEXT PRIMARY KEY,
    created_at      TEXT NOT NULL,
    customer_name   TEXT NOT NULL,
    phone           TEXT NOT NULL,
    email           TEXT NOT NULL,
    delivery_method TEXT NOT NULL CHECK (delivery_method IN ('courier', 'pickup')),
    address         TEXT NOT NULL,
    total_rub       INTEGER NOT NULL CHECK (total_rub >= 0),
    status          TEXT NOT NULL DEFAULT 'new'
);
CREATE INDEX IF NOT EXISTS idx_orders_created_at ON orders(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_orders_email ON orders(email);

CREATE TABLE IF NOT EXISTS order_items (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    order_number    TEXT NOT NULL REFERENCES orders(order_number) ON DELETE CASCADE,
    product_id      TEXT REFERENCES products(id) ON DELETE SET NULL,
    product_name    TEXT NOT NULL,
    unit_price_rub  INTEGER NOT NULL CHECK (unit_price_rub >= 0),
    quantity        INTEGER NOT NULL DEFAULT 1 CHECK (quantity > 0)
);
CREATE INDEX IF NOT EXISTS idx_order_items_order ON order_items(order_number);

CREATE TABLE IF NOT EXISTS subscribers (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    email           TEXT NOT NULL COLLATE NOCASE UNIQUE,
    subscribed_at   TEXT NOT NULL,
    consent_source  TEXT NOT NULL DEFAULT 'website'
);

-- Optional server-side storage for anonymous browser state. These tables need
-- session-cookie handlers in Go before replacing the current localStorage code.
CREATE TABLE IF NOT EXISTS browser_sessions (
    session_id      TEXT PRIMARY KEY,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    expires_at      TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS cart_items (
    session_id      TEXT NOT NULL REFERENCES browser_sessions(session_id) ON DELETE CASCADE,
    product_id      TEXT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    quantity        INTEGER NOT NULL DEFAULT 1 CHECK (quantity > 0),
    PRIMARY KEY (session_id, product_id)
);

CREATE TABLE IF NOT EXISTS favorite_items (
    session_id      TEXT NOT NULL REFERENCES browser_sessions(session_id) ON DELETE CASCADE,
    product_id      TEXT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    created_at      TEXT NOT NULL,
    PRIMARY KEY (session_id, product_id)
);

CREATE TABLE IF NOT EXISTS profiles (
    session_id      TEXT PRIMARY KEY REFERENCES browser_sessions(session_id) ON DELETE CASCADE,
    display_name    TEXT NOT NULL DEFAULT '',
    email           TEXT NOT NULL DEFAULT '',
    updated_at      TEXT NOT NULL
);

-- Initial catalog from cmd/parfume_go/main.go.
INSERT OR IGNORE INTO products
(id, name, brand, description, price_rub, tag, image_url, is_new)
VALUES
('flora-mystique', 'Flora Mystique', 'ODORIS COLLECTION', 'Цветочный · пудровый', 9500, 'Бестселлер', 'https://raw.githubusercontent.com/aurumhell/Odoris/main/assets/flora.png', 0),
('ambre-imperial', 'Ambre Impérial', 'ODORIS COLLECTION', 'Амбровый · тёплый', 11800, 'Выбор редакции', 'https://raw.githubusercontent.com/aurumhell/Odoris/main/assets/ambre.png', 1),
('oceanic-drift', 'Oceanic Drift', 'ODORIS COLLECTION', 'Морской · акватический', 8900, 'Лёгкий характер', 'https://raw.githubusercontent.com/aurumhell/Odoris/main/assets/ocean.png', 0),
('rose-de-damas', 'Rose de Damas', 'ODORIS COLLECTION', 'Роза · бархатный', 13400, 'Изысканный', 'https://raw.githubusercontent.com/aurumhell/Odoris/main/assets/roses_parf.png', 1),
('velvet-noir', 'Velvet Noir', 'ODORIS COLLECTION', 'Древесный · пряный', 14200, 'Вечерний аромат', 'https://raw.githubusercontent.com/aurumhell/Odoris/main/assets/velvet.png', 0),
('the-blanc', 'Thé Blanc', 'ODORIS COLLECTION', 'Белый чай · бергамот', 7600, 'На каждый день', 'https://raw.githubusercontent.com/aurumhell/Odoris/main/assets/tea.png', 0);

INSERT OR IGNORE INTO navigation_items (item_key, title, url, sort_order) VALUES
('catalog', 'Каталог', '/catalog', 1),
('about', 'О нас', '/about', 2),
('delivery', 'Доставка', '/delivery', 3),
('new', 'Новинки', '/new', 4);

INSERT OR IGNORE INTO advantages (id, title, body, icon, sort_order) VALUES
(1, 'Бережная доставка', 'Отправляем заказы в день покупки. Курьером до двери или в удобный пункт выдачи. Бесплатно — от 5 000 ₽.', '01', 1),
(2, 'Только оригинал', 'Работаем с проверенными поставщиками и внимательно следим за качеством каждого флакона.', '02', 2),
(3, 'Ароматный комплимент', 'Добавляем к заказу миниатюры, чтобы вы могли открыть для себя новые композиции.', '03', 3),
(4, 'Удобная оплата', 'Выбирайте комфортный способ оплаты. Данные передаются по защищённому соединению.', '04', 4);

INSERT OR IGNORE INTO pages (slug, title, description) VALUES
('about', 'О нас', 'Odoris — пространство оригинальной парфюмерии, в котором собраны выразительные композиции и ароматы с характером. Мы верим, что парфюм — это тонкий способ рассказать о себе без слов, и помогаем найти звучание, которое станет частью вашей истории.'),
('delivery', 'Доставка', 'Мы бережно упакуем ваш заказ и передадим его в доставку в день покупки. Вы можете выбрать курьера до двери или удобный пункт выдачи. При заказе от 5 000 рублей доставка бесплатная.');

INSERT OR IGNORE INTO page_points (page_slug, title, body, sort_order) VALUES
('delivery', 'Курьером', 'Доставка до двери в удобное для вас время.', 1),
('delivery', 'В пункт выдачи', 'Выберите удобный пункт при оформлении заказа.', 2),
('delivery', 'Бесплатно от 5 000 ₽', 'При заказе от указанной суммы доставка за наш счёт.', 3),
('about', 'Подлинность', 'Оригинальные ароматы от проверенных поставщиков.', 1),
('about', 'Внимание к деталям', 'Помогаем найти композицию под ваш вкус и настроение.', 2),
('about', 'Открытия', 'Знакомим с нишевой парфюмерией и культовыми брендами.', 3);

INSERT OR IGNORE INTO reviews (id, author, city, body, stars, sort_order) VALUES
(1, 'Анна', 'Москва', 'Быстрая доставка и невероятно красивая упаковка. Аромат стал моим новым фаворитом.', 5, 1),
(2, 'Мария', 'Казань', 'Помогли выбрать аромат в подарок. Очень внимательный сервис и приятные миниатюры.', 5, 2),
(3, 'Екатерина', 'Санкт-Петербург', 'Люблю открывать новые композиции. Здесь всё продумано — от каталога до получения заказа.', 5, 3),
(4, 'Ольга', 'Екатеринбург', 'Заказ приехал аккуратно упакованным. Отдельное спасибо за комплимент внутри.', 5, 4);

INSERT OR IGNORE INTO site_settings (setting_key, setting_value) VALUES
('home.hero.eyebrow', 'ОДОRIS · ИСКУССТВО АРОМАТА'),
('home.hero.title', 'Ароматы, которые вдохновляют'),
('home.hero.description', 'Оригинальная парфюмерия, нишевые композиции и культовые звучания для вашей истории.'),
('home.preview.eyebrow', 'THE ODORIS EDIT'),
('home.preview.title', 'Композиции с характером'),
('home.preview.description', 'Откройте ароматы, к которым хочется возвращаться.'),
('home.manifesto.eyebrow', 'БОЛЬШЕ, ЧЕМ АРОМАТ'),
('home.manifesto.title', 'Ваш аромат — ваша подпись.'),
('home.manifesto.description', 'Мы выбираем композиции, в которых есть настроение, глубина и индивидуальность. Чтобы каждый вдох напоминал: быть собой — уже искусство.'),
('home.advantages.title', 'Внимание — в каждой детали'),
('home.advantages.description', 'Сервис, который продолжает впечатление от любимого аромата.'),
('home.reviews.title', 'Истории наших клиентов'),
('home.reviews.description', 'Тёплые слова тех, кто уже нашёл свой аромат.'),
('home.subscribe.title', 'Письма с ароматом вдохновения'),
('home.subscribe.description', 'Новинки, особенные коллекции и немного парфюмерного искусства — без лишнего.'),
('footer.description', 'Ароматы, которые вдохновляют. Искусство быть собой.'),
('footer.email', 'hello@odoris.example');

INSERT OR IGNORE INTO site_settings (setting_key, setting_value) VALUES
('home.hero.button', 'Открыть коллекцию'),
('home.hero.secondary', 'Мир Odoris'),
('home.hero.signature', 'НАЙДИТЕ СВОЙ АРОМАТ'),
('home.preview.link', 'Весь каталог'),
('home.manifesto.title_first', 'Ваш аромат —'),
('home.manifesto.title_second', 'ваша подпись.'),
('home.manifesto.link', 'Познакомиться с Odoris'),
('home.reviews.note', 'Отзывы представлены как демонстрационный контент макета.'),
('home.subscribe.eyebrow', 'LETTERS FROM ODORIS'),
('home.subscribe.button', 'Подписаться'),
('home.subscribe.consent', 'Согласен(на) получать уведомления о новинках и акциях.'),
('home.subscribe.placeholder', 'Ваш e-mail'),
('page.cart.description', 'Ваши выбранные ароматы. Список хранится в базе данных для этой сессии.'),
('page.favorites.description', 'Композиции, к которым хочется вернуться. Избранное хранится в базе данных для этой сессии.'),
('page.profile.description', 'Сохраните контактные данные, чтобы быстрее заполнять формы при следующих посещениях.'),
('page.menu.note', 'Найдите аромат, который будет говорить за вас.'),
('layout.topline.left', 'ОДОRIS · ПАРФЮМЕРНОЕ ПРОСТРАНСТВО'),
('layout.topline.right', 'Бесплатная доставка от 5 000 ₽'),
('layout.menu.title', 'ОДОRIS · НАВИГАЦИЯ'),
('layout.footer.tagline', 'Ароматы, которые вдохновляют. Искусство быть собой.'),
('layout.footer.contact_title', 'Остаёмся на связи'),
('layout.footer.contact_text', 'Вопросы о выборе аромата и заказе — мы рядом.'),
('layout.footer.email', 'hello@odoris.example'),
('layout.footer.explore', 'Исследовать'),
('layout.footer.help', 'Помощь'),
('layout.footer.copyright', 'Все права защищены.'),
('profile.saved', 'Контактные данные сохранены в базе данных.');
INSERT OR IGNORE INTO site_settings (setting_key, setting_value) VALUES
('home.hero.title_first', 'Ароматы,'),
('home.hero.title_second', 'которые вдохновляют');
