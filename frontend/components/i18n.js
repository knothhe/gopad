/* Message IDs and parameter interpolation are shared by static and live UI. */
(() => {
  let locale = 'en', language = 'system', messages = {}, generation = 0;
  let metadata = {appName:'gopad',shortcut:'',version:'',channel:''};
  const formatValue = value => typeof value === 'number' ? new Intl.NumberFormat(locale).format(value) : value && typeof value === 'object' && typeof value.message === 'string' ? value.message : String(value ?? '');
  const t = (key,variables = {}) => { const params = {name:metadata.appName,...variables}; return (messages[key] || key).replace(/\{([A-Za-z][A-Za-z0-9_]*)\}/g,(_match,name) => Object.prototype.hasOwnProperty.call(params,name) ? formatValue(params[name]) : `{${name}}`); };
  function applyElement(element) {
    const params = element._i18nParams || {};
    if (element.dataset.i18nMeta) element.textContent = metadata[element.dataset.i18nMeta] || '—';
    if (element.dataset.i18n) element.textContent = t(element.dataset.i18n,params);
    for (const attribute of ['aria-label','title','placeholder']) {
      const key = element.getAttribute(`data-i18n-${attribute}`);
      if (key) element.setAttribute(attribute,t(key,params));
    }
  }
  function apply(root = document) {
    if (root instanceof Element) applyElement(root);
    root.querySelectorAll('[data-i18n-meta],[data-i18n],[data-i18n-aria-label],[data-i18n-title],[data-i18n-placeholder]').forEach(applyElement);
    document.documentElement.lang = locale;
  }
  function setText(element,key,params = {}) { element.dataset.i18n = key; element._i18nParams = params; applyElement(element); }
  async function load() {
    const current = ++generation;
    const response = await fetch('/api/i18n');
    if (!response.ok) throw new Error('I18N_MESSAGES_UNAVAILABLE');
    const data = await response.json();
    if (!data.messages || typeof data.messages !== 'object') throw new Error('I18N_MESSAGES_UNAVAILABLE');
    if (current !== generation) return;
    const nextLocale = data.locale === 'zh-CN' ? 'zh-CN' : 'en';
    const nextMetadata = {appName:data.appName || 'gopad',shortcut:data.shortcut || '',version:data.version || '',channel:data.channel || ''};
    const changed = locale !== nextLocale || JSON.stringify(messages) !== JSON.stringify(data.messages) || JSON.stringify(metadata) !== JSON.stringify(nextMetadata);
    metadata = nextMetadata; locale = nextLocale; language = typeof data.language === 'string' ? data.language : 'system'; messages = data.messages;
    apply();
    if (changed) document.dispatchEvent(new CustomEvent('gopad:languagechange'));
  }
  window.GopadI18n = {t,apply,setText,load,has:key => Object.prototype.hasOwnProperty.call(messages,key),get locale(){return locale},get language(){return language},get metadata(){return {...metadata}}};
})();
