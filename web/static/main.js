(() => {
    const toggle = document.querySelector('.menu-toggle');
    const menu = document.querySelector('#site-menu');
    const closeTriggers = document.querySelectorAll('[data-menu-close]');
    const toast = document.querySelector('#site-toast');
    const LEGACY_CART_KEY = 'odoris-cart-v1';
    const LEGACY_FAVORITES_KEY = 'odoris-favorites-v1';
    const LEGACY_PROFILE_KEY = 'odoris-profile-v1';

    let state = { cart: [], favorites: [], profile: { name: '', email: '' } };
    let toastTimeout;

    const readLegacyList = (key) => {
        try {
            const value = JSON.parse(localStorage.getItem(key) || '[]');
            return Array.isArray(value) ? [...new Set(value.filter((item) => typeof item === 'string'))] : [];
        } catch { return []; }
    };
    const readLegacyProfile = () => {
        try {
            const value = JSON.parse(localStorage.getItem(LEGACY_PROFILE_KEY) || 'null');
            return value && typeof value === 'object' ? { name: String(value.name || ''), email: String(value.email || '') } : { name: '', email: '' };
        } catch { return { name: '', email: '' }; }
    };

    const showToast = (message) => {
        if (!toast) return;
        toast.textContent = message;
        toast.classList.add('is-visible');
        window.clearTimeout(toastTimeout);
        toastTimeout = window.setTimeout(() => toast.classList.remove('is-visible'), 2600);
    };

    const persistState = async () => {
        const response = await fetch('/api/state', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'same-origin',
            body: JSON.stringify(state)
        });
        if (!response.ok) throw new Error('Не удалось сохранить данные в базе данных.');
        state = await response.json();
        return state;
    };

    const loadState = async () => {
        const response = await fetch('/api/state', { credentials: 'same-origin', cache: 'no-store' });
        if (!response.ok) throw new Error('Не удалось загрузить данные пользователя.');
        state = await response.json();
        const oldCart = readLegacyList(LEGACY_CART_KEY);
        const oldFavorites = readLegacyList(LEGACY_FAVORITES_KEY);
        const oldProfile = readLegacyProfile();
        const hasLegacy = oldCart.length || oldFavorites.length || oldProfile.name || oldProfile.email;
        const databaseIsEmpty = !state.cart.length && !state.favorites.length && !state.profile.name && !state.profile.email;
        if (hasLegacy && databaseIsEmpty) {
            state.cart = oldCart;
            state.favorites = oldFavorites;
            state.profile = oldProfile;
            await persistState();
        }
        try {
            localStorage.removeItem(LEGACY_CART_KEY);
            localStorage.removeItem(LEGACY_FAVORITES_KEY);
            localStorage.removeItem(LEGACY_PROFILE_KEY);
        } catch { /* Browser storage can be disabled. */ }
        if (new URLSearchParams(window.location.search).get('order') === 'success') {
            state.cart = [];
            await persistState();
        }
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
        document.querySelectorAll('[data-cart-count]').forEach((node) => { node.textContent = String(state.cart.length); node.hidden = state.cart.length === 0; });
        document.querySelectorAll('[data-favorites-count]').forEach((node) => { node.textContent = String(state.favorites.length); node.hidden = state.favorites.length === 0; });
    };

    const updateProductButtons = () => {
        document.querySelectorAll('[data-product-id]').forEach((card) => {
            const id = card.dataset.productId;
            const favoriteButton = card.querySelector('[data-favorite]');
            const cartButton = card.querySelector('[data-add-cart]');
            const isFavorite = state.favorites.includes(id);
            const inCart = state.cart.includes(id);
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
        const selected = type === 'cart' ? state.cart : state.favorites;
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

    const refreshUI = () => { updateCounters(); updateProductButtons(); updateCollection(); };

    document.addEventListener('click', async (event) => {
        const favoriteButton = event.target.closest('[data-favorite]');
        if (favoriteButton) {
            const card = favoriteButton.closest('[data-product-id]');
            if (!card) return;
            const id = card.dataset.productId;
            state.favorites = state.favorites.includes(id) ? state.favorites.filter((item) => item !== id) : [...state.favorites, id];
            refreshUI();
            try { await persistState(); showToast(state.favorites.includes(id) ? 'Добавлено в избранное' : 'Удалено из избранного'); }
            catch (error) { showToast(error.message); }
            return;
        }
        const cartButton = event.target.closest('[data-add-cart]');
        if (cartButton) {
            const card = cartButton.closest('[data-product-id]');
            if (!card) return;
            const id = card.dataset.productId;
            const collection = document.querySelector('[data-collection-page]')?.dataset.collectionPage;
            if (collection === 'cart' || state.cart.includes(id)) {
                state.cart = state.cart.filter((item) => item !== id);
                showToast('Аромат удалён из корзины');
            } else {
                state.cart = [...state.cart, id];
                showToast('Аромат добавлен в корзину');
            }
            refreshUI();
            try { await persistState(); }
            catch (error) { showToast(error.message); }
        }
    });

    const profileForm = document.querySelector('#profile-form');
    if (profileForm) {
        const nameField = profileForm.querySelector('[name="name"]');
        const emailField = profileForm.querySelector('[name="email"]');
        const status = document.querySelector('#profile-status');
        profileForm.addEventListener('submit', async (event) => {
            event.preventDefault();
            state.profile = { name: nameField.value.trim(), email: emailField.value.trim() };
            try {
                await persistState();
                status.textContent = 'Контактные данные сохранены в базе данных.';
            } catch (error) { status.textContent = error.message; }
        });
    }

    loadState().then(() => {
        if (profileForm) {
            profileForm.querySelector('[name="name"]').value = state.profile.name || '';
            profileForm.querySelector('[name="email"]').value = state.profile.email || '';
        }
        refreshUI();
    }).catch((error) => {
        refreshUI();
        showToast(error.message);
        console.error(error);
    });
})();
