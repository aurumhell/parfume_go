(() => {
    const toggle = document.querySelector('.menu-toggle');
    const menu = document.querySelector('#site-menu');
    const closeTriggers = document.querySelectorAll('[data-menu-close]');
    const toast = document.querySelector('#site-toast');
    const CART_KEY = 'odoris-cart-v1';
    const FAVORITES_KEY = 'odoris-favorites-v1';
    const PROFILE_KEY = 'odoris-profile-v1';

    const readList = (key) => {
        try {
            const value = JSON.parse(localStorage.getItem(key) || '[]');
            return Array.isArray(value) ? [...new Set(value.filter((item) => typeof item === 'string'))] : [];
        } catch {
            return [];
        }
    };
    const writeList = (key, list) => {
        try { localStorage.setItem(key, JSON.stringify(list)); } catch { /* Storage may be disabled. */ }
    };

    if (new URLSearchParams(window.location.search).get('order') === 'success') {
        writeList(CART_KEY, []);
    }
    let cart = readList(CART_KEY);
    let favorites = readList(FAVORITES_KEY);
    let toastTimeout;

    const showToast = (message) => {
        if (!toast) return;
        toast.textContent = message;
        toast.classList.add('is-visible');
        window.clearTimeout(toastTimeout);
        toastTimeout = window.setTimeout(() => toast.classList.remove('is-visible'), 2600);
    };

    const setMenuState = (isOpen) => {
        if (!toggle || !menu) return;
        document.body.classList.toggle('menu-open', isOpen);
        toggle.setAttribute('aria-expanded', String(isOpen));
        toggle.setAttribute('aria-label', isOpen ? 'Закрыть меню' : 'Открыть меню');
        menu.setAttribute('aria-hidden', String(!isOpen));
        menu.inert = !isOpen;
        if (isOpen) menu.querySelector('.menu-close')?.focus();
        else toggle.focus();
    };

    if (toggle && menu) {
        toggle.addEventListener('click', () => setMenuState(toggle.getAttribute('aria-expanded') !== 'true'));
        closeTriggers.forEach((trigger) => trigger.addEventListener('click', () => setMenuState(false)));
        menu.querySelectorAll('a').forEach((link) => link.addEventListener('click', () => setMenuState(false)));
        document.addEventListener('keydown', (event) => {
            if (event.key === 'Escape' && toggle.getAttribute('aria-expanded') === 'true') setMenuState(false);
        });
    }

    const updateCounters = () => {
        document.querySelectorAll('[data-cart-count]').forEach((node) => { node.textContent = String(cart.length); node.hidden = cart.length === 0; });
        document.querySelectorAll('[data-favorites-count]').forEach((node) => { node.textContent = String(favorites.length); node.hidden = favorites.length === 0; });
    };

    const updateProductButtons = () => {
        document.querySelectorAll('[data-product-id]').forEach((card) => {
            const id = card.dataset.productId;
            const favoriteButton = card.querySelector('[data-favorite]');
            const cartButton = card.querySelector('[data-add-cart]');
            const isFavorite = favorites.includes(id);
            const inCart = cart.includes(id);
            if (favoriteButton) {
                favoriteButton.setAttribute('aria-pressed', String(isFavorite));
                favoriteButton.classList.toggle('is-active', isFavorite);
                favoriteButton.textContent = isFavorite ? '♥' : '♡';
                favoriteButton.setAttribute('aria-label', `${isFavorite ? 'Убрать' : 'Добавить'} ${id} ${isFavorite ? 'из избранного' : 'в избранное'}`);
            }
            if (cartButton) {
                cartButton.classList.toggle('is-added', inCart);
                cartButton.innerHTML = inCart ? 'В корзине <span>✓</span>' : 'В корзину <span>+</span>';
            }
        });
    };

    const updateCollection = () => {
        const root = document.querySelector('[data-collection-page]');
        if (!root) return;
        const type = root.dataset.collectionPage;
        const selected = type === 'cart' ? cart : favorites;
        const cards = [...root.querySelectorAll('[data-product-id]')];
        cards.forEach((card) => {
            const visible = selected.includes(card.dataset.productId);
            card.hidden = !visible;
            const cartButton = card.querySelector('[data-add-cart]');
            if (type === 'cart' && cartButton) {
                cartButton.innerHTML = 'Убрать <span>×</span>';
                cartButton.setAttribute('aria-label', `Убрать ${card.querySelector('h3')?.textContent || 'аромат'} из корзины`);
            }
            if (type === 'favorites' && card.querySelector('[data-favorite]')) {
                card.querySelector('[data-favorite]').setAttribute('aria-label', 'Убрать из избранного');
            }
        });
        const empty = root.querySelector('[data-collection-empty]');
        if (empty) empty.hidden = selected.length !== 0;
        const count = root.querySelector('[data-collection-count]');
        if (count) count.textContent = `${selected.length} ${selected.length === 1 ? 'аромат' : selected.length > 1 && selected.length < 5 ? 'аромата' : 'ароматов'}`;
        const total = root.querySelector('[data-cart-total]');
        if (total) {
            const sum = cards.filter((card) => selected.includes(card.dataset.productId)).reduce((value, card) => value + Number(card.dataset.priceValue || 0), 0);
            total.textContent = `${new Intl.NumberFormat('ru-RU').format(sum)} ₽`;
        }
        const checkoutForm = root.querySelector('#checkout-form');
        const checkoutProducts = root.querySelector('#checkout-products');
        if (checkoutForm) checkoutForm.hidden = selected.length === 0;
        if (checkoutProducts) checkoutProducts.value = selected.join(',');
    };

    document.addEventListener('click', (event) => {
        const favoriteButton = event.target.closest('[data-favorite]');
        if (favoriteButton) {
            const card = favoriteButton.closest('[data-product-id]');
            if (!card) return;
            const id = card.dataset.productId;
            favorites = favorites.includes(id) ? favorites.filter((item) => item !== id) : [...favorites, id];
            writeList(FAVORITES_KEY, favorites);
            updateCounters(); updateProductButtons(); updateCollection();
            showToast(favorites.includes(id) ? 'Добавлено в избранное' : 'Удалено из избранного');
            return;
        }

        const cartButton = event.target.closest('[data-add-cart]');
        if (cartButton) {
            const card = cartButton.closest('[data-product-id]');
            if (!card) return;
            const id = card.dataset.productId;
            const collection = document.querySelector('[data-collection-page]')?.dataset.collectionPage;
            if (collection === 'cart' || cart.includes(id)) {
                cart = cart.filter((item) => item !== id);
                showToast('Аромат удалён из корзины');
            } else {
                cart = [...cart, id];
                showToast('Аромат добавлен в корзину');
            }
            writeList(CART_KEY, cart);
            updateCounters(); updateProductButtons(); updateCollection();
        }
    });

    const profileForm = document.querySelector('#profile-form');
    if (profileForm) {
        const nameField = profileForm.querySelector('[name="name"]');
        const emailField = profileForm.querySelector('[name="email"]');
        const status = document.querySelector('#profile-status');
        try {
            const profile = JSON.parse(localStorage.getItem(PROFILE_KEY) || 'null');
            if (profile) { nameField.value = profile.name || ''; emailField.value = profile.email || ''; }
        } catch { /* Ignore invalid local profile data. */ }
        profileForm.addEventListener('submit', (event) => {
            event.preventDefault();
            const profile = { name: nameField.value.trim(), email: emailField.value.trim() };
            try {
                localStorage.setItem(PROFILE_KEY, JSON.stringify(profile));
                status.textContent = 'Данные сохранены на этом устройстве.';
            } catch {
                status.textContent = 'Браузер не разрешил сохранить данные. Проверьте настройки хранилища.';
            }
        });
    }

    updateCounters();
    updateProductButtons();
    updateCollection();
})();
