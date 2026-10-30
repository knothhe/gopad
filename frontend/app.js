const paths = {
  grid: '<rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="3" y="14" width="7" height="7" rx="1.5"/><rect x="14" y="14" width="7" height="7" rx="1.5"/>',
  star: '<path d="m12 3 2.8 5.7 6.2.9-4.5 4.4 1.1 6.2-5.6-3-5.6 3 1.1-6.2L3 9.6l6.2-.9Z"/>',
  command:
    '<path d="M9 7V4.5A2.5 2.5 0 1 0 6.5 7H17.5A2.5 2.5 0 1 0 15 4.5v15a2.5 2.5 0 1 0 2.5-2.5h-11A2.5 2.5 0 1 0 9 19.5Z"/>',
  "eye-off":
    '<path d="m3 3 18 18M10.6 10.6a2 2 0 0 0 2.8 2.8M9.9 5.2A10 10 0 0 1 12 5c6 0 10 7 10 7a18 18 0 0 1-3.1 3.9M6.5 6.5A20 20 0 0 0 2 12s4 7 10 7a11 11 0 0 0 5.5-1.5"/>',
  eye: '<path d="M2 12s4-7 10-7 10 7 10 7-4 7-10 7S2 12 2 12Z"/><circle cx="12" cy="12" r="3"/>',
  settings:
    '<path d="m9.5 3-.6 2.4-2 .9-2.2-.7L2.2 10l1.7 1.7v2.2L2.2 15.5l2.5 4.3 2.3-.6 1.9 1 .6 2.3h5l.6-2.4 2-.9 2.2.7 2.5-4.4-1.7-1.6v-2.2l1.7-1.7-2.5-4.3-2.3.6-1.9-1L14.5 3Z" transform="translate(0 -1) scale(1 .96)"/><circle cx="12" cy="12" r="3"/>',
  layout:
    '<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M8 4v16"/>',
  refresh:
    '<path d="M20 7v5h-5M4 17v-5h5"/><path d="M6 7a7 7 0 0 1 11.5-1L20 9M4 15l2.5 3A7 7 0 0 0 18 17"/>',
  expand: '<path d="M8 3H3v5m13-5h5v5M3 16v5h5m13-5v5h-5"/>',
  x: '<path d="m6 6 12 12M18 6 6 18"/>',
  search: '<circle cx="10.8" cy="10.8" r="7"/><path d="m16 16 4.5 4.5"/>',
  sort: '<path d="M8 4v16m-4-4 4 4 4-4M15 5h6m-6 5h4m-4 5h2"/>',
  "chevron-down": '<path d="m7 10 5 5 5-5"/>',
  sliders:
    '<path d="M4 7h4m4 0h8M4 17h8m4 0h4"/><circle cx="10" cy="7" r="2"/><circle cx="14" cy="17" r="2"/>',
  mouse: '<rect x="6" y="2" width="12" height="20" rx="6"/><path d="M12 6v4"/>',
  "image-small":
    '<rect x="5" y="5" width="14" height="14" rx="3"/><path d="m5 15 4-4 3 3 2-2 5 5"/>',
  "image-large":
    '<rect x="3" y="3" width="18" height="18" rx="4"/><path d="m3 16 5-5 4 4 3-3 6 7"/><circle cx="15" cy="8" r="1"/>',
  "arrow-right": '<path d="M4 12h16m-6-6 6 6-6 6"/>',
  more: '<circle cx="5" cy="12" r="1"/><circle cx="12" cy="12" r="1"/><circle cx="19" cy="12" r="1"/>',
  folder:
    '<path d="M3 7V5a2 2 0 0 1 2-2h5l3 3h6a2 2 0 0 1 2 2v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2Z"/>',
  copy: '<rect x="8" y="8" width="12" height="13" rx="2"/><path d="M16 8V4a2 2 0 0 0-2-2H4a2 2 0 0 0-2 2v10a2 2 0 0 0 2 2h4"/>',
  launch:
    '<path d="M14 3h7v7m0-7L10 14"/><path d="M10 3H5a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-5"/>',
  minus: '<path d="M6 12h12"/>',
  check: '<path d="m5 12 4 4L19 6"/>',
  info: '<circle cx="12" cy="12" r="9"/><path d="M12 11v6m0-10v.1"/>',
};
Object.assign(paths, {
  plus: '<path d="M12 5v14M5 12h14"/>',
  up: '<path d="m6 14 6-6 6 6"/>',
  down: '<path d="m6 10 6 6 6-6"/>',
  grip: '<path d="M8 5h.01M16 5h.01M8 12h.01M16 12h.01M8 19h.01M16 19h.01" stroke-width="3"/>',
  pencil: '<path d="m15 5 4 4M4 20l4-1L20 7a2.8 2.8 0 0 0-4-4L4 15Z"/>',
  trash: '<path d="M3 6h18M9 6V3h6v3M6 6l1 15h10l1-15M10 10v7M14 10v7"/>',
});
const icon = name => `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${paths[name] || paths.grid}</svg>`;
const $ = selector => document.querySelector(selector);
const {t,setText: localizedText} = window.GopadI18n;
const htmlText = (key,params) => escapeHTML(t(key,params));
class AppError extends Error {
  constructor(key,params = {},detail) { super(); this.key = key; this.params = params; this.detail = detail; Object.defineProperty(this,'message',{get:() => t(key,params)}); }
}
const {GlassDialog, CategoryTabs, reducedMotion, isComposing, gridInlineInset, duration: motionDuration} = window.GopadUI;
document.querySelectorAll('[data-icon]').forEach(el => { el.innerHTML = icon(el.dataset.icon); });
const grid = $('#app-grid'), search = $('#search'), menu = $('#context-menu');
const settings = new GlassDialog($('#settings-dialog'));
const categoriesDialog = new GlassDialog($('#categories-dialog'));
const nameDialog = new GlassDialog($('#category-name-dialog'));
const picker = new GlassDialog($('#picker-dialog'));
const isNative = !!window.mygo;
const BUILTINS = {all:'web.category.all',favorites:'web.category.favorites',system:'web.category.system',hidden:'web.category.hidden'};
const builtin = id => Object.prototype.hasOwnProperty.call(BUILTINS,id);
const defaultCategories = () => Object.keys(BUILTINS).map(id => ({id,name:id,appIds:[],visible:id !== 'hidden'}));
const defaults = () => ({hidden:[],order:[],favorites:[],iconSize:80,language:'system',lastCategory:'all',categories:defaultCategories()});
const clone = value => typeof structuredClone === 'function' ? structuredClone(value) : JSON.parse(JSON.stringify(value));
const unique = values => [...new Set((Array.isArray(values) ? values : []).filter(value => typeof value === 'string' && value))];
const escapeHTML = value => String(value ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'})[c]);
const normalize = value => String(value || '').normalize('NFKC').toLocaleLowerCase().replace(/\s/g,'');
function normalizePreferences(raw = {}) {
  const seen = new Set(), categories = [];
  for (const item of raw.categories || []) {
    if (!item || typeof item.id !== 'string') continue;
    const id = item.id.trim(), isBuiltin = builtin(id);
    if (!/^[A-Za-z0-9_-]{1,80}$/.test(id) || seen.has(id)) continue;
    if (!isBuiltin && categories.filter(c => !builtin(c.id)).length >= 64) continue;
    const name = isBuiltin ? String(item.name || id) : Array.from(String(item.name || '').replace(/[\u0000-\u001f\u007f-\u009f]/g,'').trim()).slice(0,40).join('').trim();
    if (!name) continue;
    seen.add(id);
    categories.push({id,name,appIds:isBuiltin ? [] : unique(item.appIds),visible:item.visible === true});
  }
  for (const item of defaultCategories()) if (!seen.has(item.id)) categories.push(item);
  if (!categories.some(item => item.visible)) categories.find(item => item.id === 'all').visible = true;
  return {...raw,language:typeof raw.language === 'string' && raw.language ? raw.language : 'system',lastCategory:typeof raw.lastCategory === 'string' ? raw.lastCategory : 'all',hidden:unique(raw.hidden),order:unique(raw.order),favorites:unique(raw.favorites),iconSize:Math.max(48,Math.min(112,Number(raw.iconSize)||80)),categories};
}
let state = {apps:[],preferences:defaults()};
let lastConfirmedPreferences = defaults();
let currentView = 'all', temporaryView = null, sortMode = 'custom', query = '', selectedID = null;
let draggingID = null, draggedCategoryID = null, suppressClickUntil = 0;
let toastTimer, toastState = null, translationRevision = 0, menuApp = null, lastAppFocus = null, loading = true, lastRefresh = Date.now(), closingManager = false;
let editingCategoryID = null, pickerState = null, nameSession = 0, pickerSession = 0;
const pendingSaves = [];
let saving = false, saveFailures = 0, closePreparation = null;
const appByID = id => state.apps.find(app => app.id === id);
const categoryByID = id => state.preferences.categories.find(category => category.id === id);
const categoryName = category => category ? builtin(category.id) ? t(BUILTINS[category.id]) : category.name : '';
const appCount = count => t(count === 1 ? 'web.appCountOne' : 'web.appCount',{count});
const isSystem = app => app.kind === 'system' || app.path.startsWith('/System/');
const visibleCategories = () => state.preferences.categories.filter(category => category.visible);

async function request(path, method = 'GET', body) {
  let response;
  try { response = await fetch(`/api/${path}`,{method,headers:body ? {'Content-Type':'application/json'} : {},body:body ? JSON.stringify(body) : undefined}); }
  catch (err) { throw new AppError('web.requestFailed',{},err.message); }
  let data;
  try { data = await response.json(); } catch { throw new AppError('web.requestDataError'); }
  if (!response.ok) throw new AppError(data.errorKey && window.GopadI18n.has(data.errorKey) ? data.errorKey : 'web.requestFailed',data.errorParams || {},data.detail || data.error);
  return data;
}
function renderToast() {
  if (!toastState) return;
  const el = $('#toast'), {key,params,action} = toastState;
  el.replaceChildren(document.createTextNode(t(key,params)));
  if (action) {
    const button = document.createElement('button'); button.textContent = t(action.label);
    button.onclick = () => { el.hidden = true; action.run(); }; el.append(button);
  }
}
function toast(key,params = {},action) {
  clearTimeout(toastTimer);
  const el = $('#toast'), top = GlassDialog.top();
  (top?.element || document.body).append(el);
  el.classList.toggle('dialog-toast',!!top);
  toastState = {key,params,action}; renderToast(); el.hidden = false;
  toastTimer = setTimeout(() => { el.hidden = true; },action ? 6500 : 4000);
}
function reportError(err,key = 'web.actionError') { console.error(err); toast(key,{error:err}); }
function replayPending() {
  const preferences = clone(lastConfirmedPreferences);
  for (const entry of pendingSaves) entry.change(preferences);
  state.preferences = normalizePreferences(preferences);
  setSize(state.preferences.iconSize);
  render();
}
async function drainSaves() {
  if (saving) return;
  saving = true;
  while (pendingSaves.length) {
    const entry = pendingSaves[0];
    const snapshot = clone(lastConfirmedPreferences);
    entry.change(snapshot);
    let ok = false;
    const previousLanguage = lastConfirmedPreferences.language;
    try {
      const response = await request('preferences','PUT',normalizePreferences(snapshot));
      lastConfirmedPreferences = normalizePreferences(response.preferences || snapshot);
      ok = true;
    } catch (err) { saveFailures++; reportError(err,'web.saveError'); }
    pendingSaves.shift();
    replayPending();
    if (ok && previousLanguage !== lastConfirmedPreferences.language && state.preferences.language === lastConfirmedPreferences.language) await refreshTranslations({report:true});
    entry.resolve(ok);
  }
  saving = false;
}
function updatePreferences(change) {
  let resolve;
  const promise = new Promise(done => { resolve = done; });
  pendingSaves.push({change,resolve,promise});
  replayPending();
  drainSaves();
  return promise;
}
async function persist() {
  let ok = true;
  while (pendingSaves.length) {
    const results = await Promise.all(pendingSaves.map(entry => entry.promise));
    if (!results.every(Boolean)) ok = false;
  }
  return ok;
}
function cancelClosePreparation() {
  if (!closePreparation) return;
  document.body.inert = closePreparation.inertBefore;
  document.body.classList.remove('leaving');
  closePreparation = null;
  closingManager = false;
}
function prepareClose() {
  if (closePreparation) return closePreparation.promise;
  const preparation = {inertBefore:!!document.body.inert,promise:null}, failuresBefore = saveFailures;
  closePreparation = preparation;
  document.body.inert = true;
  closeMenu();
  preparation.promise = (async () => {
    try {
      if (!(await persist())) {
        if (closePreparation === preparation) cancelClosePreparation();
        return false;
      }
      if (closePreparation !== preparation) return false;
      document.body.classList.add('leaving');
      await new Promise(resolve => setTimeout(resolve,motionDuration()));
      const saved = await persist();
      if (closePreparation !== preparation) return false;
      if (!saved || saveFailures !== failuresBefore) { cancelClosePreparation(); return false; }
      return true;
    } catch (err) {
      if (closePreparation === preparation) cancelClosePreparation();
      throw err;
    }
  })();
  return preparation.promise;
}
window.__gopadPrepareClose = prepareClose;
window.__gopadCancelClose = cancelClosePreparation;
function orderedApps() {
  if (sortMode === 'name') return [...state.apps].sort((a,b) => a.name.localeCompare(b.name,window.GopadI18n.locale,{numeric:true,sensitivity:'base'}));
  const ranks = new Map(state.preferences.order.map((id,index) => [id,index]));
  // Go supplies the stable baseline. Unranked apps retain that API order.
  return [...state.apps].sort((a,b) => {
    const ar = ranks.get(a.id), br = ranks.get(b.id);
    if (ar !== undefined && br !== undefined) return ar-br;
    if (ar !== undefined) return -1;
    if (br !== undefined) return 1;
    return 0;
  });
}
function categoryApps(category,ordered = true) {
  if (!category) return [];
  const hidden = new Set(state.preferences.hidden), favorite = new Set(state.preferences.favorites), members = new Set(category.appIds);
  return (ordered ? orderedApps() : state.apps).filter(app => {
    if (category.id === 'hidden') return hidden.has(app.id);
    if (hidden.has(app.id)) return false;
    if (category.id === 'favorites') return favorite.has(app.id);
    if (category.id === 'system') return isSystem(app);
    return category.id === 'all' || members.has(app.id);
  });
}
function visibleApps() {
  return categoryApps(categoryByID(currentView)).filter(app => !query || normalize(`${app.name} ${app.bundleId || ''} ${app.path.split('/').pop()}`).includes(normalize(query)));
}
function setSize(value, save = false) {
  const size = Math.max(48,Math.min(112,Number(value)||80));
  if (save) return updatePreferences(preferences => { preferences.iconSize = size; });
  document.documentElement.style.setProperty('--icon-size',`${size}px`);
  document.documentElement.style.setProperty('--cell-width',`${Math.max(104,size+52)}px`);
  document.documentElement.style.setProperty('--cell-height',`${size+59}px`);
  $('#icon-size').value = $('#settings-icon-size').value = size;
  $('#icon-size-value').textContent = size;
  $('#settings-size-value').textContent = `${size} px`;
}
const tabs = new CategoryTabs($('#navigation'),{
  select:id => changeView(id,{temporary:true}),
  reorder:reorderCategory,
  canDropApp:id => !!draggingID && !builtin(id),
  dropApp:id => {
    if (!draggingID || builtin(id)) return;
    const appID = draggingID, category = categoryByID(id);
    updatePreferences(preferences => {
      const item = preferences.categories.find(item => item.id === id);
      if (item) item.appIds = unique([...item.appIds,appID]);
    });
    draggingID = null;
    toast('web.toast.addedCategory',{name:categoryName(category)});
  },
});
function ensureCurrentCategory() {
  const current = categoryByID(currentView);
  if (!current || (!current.visible && temporaryView !== current.id)) { currentView = visibleCategories()[0].id; temporaryView = null; selectedID = null; query = ''; search.value = ''; syncSearch(); $('#apps-scroll').scrollTop = 0; }
  if (categoryByID(currentView).visible) temporaryView = null;
}
function render() {
  ensureCurrentCategory();
  const categories = state.preferences.categories.filter(item => item.visible || item.id === temporaryView);
  const navigation = $('#navigation'), showTabs = visibleCategories().length > 1;
  const focusedNavigation = navigation.contains(document.activeElement);
  navigation.hidden = !showTabs;
  tabs.render(categories.map(category => ({...category,name:categoryName(category)})),currentView,item => categoryApps(item,false).length);
  if (!showTabs && focusedNavigation) search.focus({preventScroll:true});
  renderLanguageSelector();
  const panel = $('#apps-scroll');
  panel.removeAttribute('data-i18n-aria-label');
  if (showTabs) {
    panel.setAttribute('aria-labelledby',`category-tab-${currentView}`);
    panel.removeAttribute('aria-label');
  } else {
    panel.removeAttribute('aria-labelledby');
    panel.setAttribute('aria-label',categoryName(categoryByID(currentView)));
  }
  $('#category-shortcut').hidden = !showTabs;
  const hiddenCount = state.apps.filter(app => state.preferences.hidden.includes(app.id)).length;
  $('#hidden-summary').textContent = hiddenCount;
  const apps = visibleApps();
  $('#collection-count').removeAttribute('data-i18n'); $('#collection-count').textContent = appCount(apps.length);
  const focusedID = document.activeElement?.closest('.app-tile')?.dataset.id;
  const existing = new Map([...grid.children].map(tile => [tile.dataset.id,tile]));
  const positions = new Map([...existing].map(([id,tile]) => [id,tile.getBoundingClientRect()]));
  const tiles = apps.map(app => {
    const revision = JSON.stringify([app.name,app.path,app.version,state.preferences.hidden.includes(app.id),state.preferences.favorites.includes(app.id),currentView === 'hidden',translationRevision]);
    let tile = existing.get(app.id);
    if (!tile || tile.dataset.revision !== revision) { tile = createTile(app); tile.dataset.revision = revision; }
    tile.classList.toggle('keyboard-selected',app.id === selectedID);
    return tile;
  });
  grid.replaceChildren(...tiles);
  if (!reducedMotion()) tiles.forEach(tile => {
    if (typeof tile.animate !== 'function') return;
    const before = positions.get(tile.dataset.id), after = tile.getBoundingClientRect();
    if (before?.width && after.width) {
      const dx = before.left-after.left, dy = before.top-after.top;
      if (Math.abs(dx)>1 || Math.abs(dy)>1) tile.animate([{transform:`translate(${dx}px,${dy}px)`},{transform:'translate(0,0)'}],{duration:200,easing:'cubic-bezier(.2,.65,.25,1)'});
    } else if (after.width) tile.animate([{opacity:0,transform:'translateY(5px)'},{opacity:1,transform:'translateY(0)'}],{duration:200,easing:'cubic-bezier(.2,.65,.25,1)'});
  });
  const empty = $('#empty-state'); empty.hidden = apps.length > 0;
  if (!apps.length) {
    const custom = !builtin(currentView);
    const title = query ? t('web.empty.query',{query:search.value}) : custom ? categoryName(categoryByID(currentView)) : t(`web.empty.${currentView}`);
    const action = query ? ['clear','web.clearSearch'] : custom ? ['add-apps','web.addApps'] : ['all','web.allApps'];
    empty.innerHTML = `<div class="empty-illustration">${icon(query ? 'search' : custom ? 'folder' : currentView === 'favorites' ? 'star' : currentView === 'hidden' ? 'eye-off' : 'grid')}</div><h2>${escapeHTML(title)}</h2><button class="button glass-surface glass-control" data-empty-action="${action[0]}">${htmlText(action[1])}</button>`;
  }
  if (focusedID) [...grid.children].find(tile => tile.dataset.id === focusedID)?.focus({preventScroll:true});
  if (categoriesDialog.open) renderCategoryList();
  if (picker.open && pickerState) renderPicker();
  syncGridAlignment();
}
function syncGridAlignment() {
  const width = grid.clientWidth;
  if (!width) return;
  const rootStyle = getComputedStyle(document.documentElement), gridStyle = getComputedStyle(grid);
  const inset = gridInlineInset(width,parseFloat(rootStyle.getPropertyValue('--cell-width')) || 132,parseFloat(rootStyle.getPropertyValue('--icon-size')) || 80,parseFloat(gridStyle.columnGap) || 0);
  const scrollbar = Math.max(0,($('#apps-scroll').offsetWidth || width)-width);
  const values = {'--grid-inline-inset':inset,'--grid-inline-end-inset':inset+scrollbar};
  for (const [property,value] of Object.entries(values)) {
    const pixels = `${value.toFixed(3)}px`;
    if (document.documentElement.style.getPropertyValue(property) !== pixels) document.documentElement.style.setProperty(property,pixels);
  }
}
window.addEventListener('resize',syncGridAlignment);
if (typeof ResizeObserver !== 'undefined') new ResizeObserver(syncGridAlignment).observe(grid);
function createTile(app) {
  const tile = document.createElement('div');
  tile.className = 'app-tile'; tile.dataset.id = app.id; tile.tabIndex = 0; tile.setAttribute('role','listitem'); tile.draggable = true;
  tile.setAttribute('aria-label',state.preferences.hidden.includes(app.id) ? t('web.appHiddenLabel',{name:app.name}) : app.name);
  tile.title = `${app.name}\n${app.path}`;
  const img = document.createElement('img'); img.className = 'app-icon'; img.src = `/api/icon/${encodeURIComponent(app.id)}`; img.alt = ''; img.draggable = false; img.loading = 'lazy';
  img.onerror = () => { const fallback = document.createElement('span'); fallback.className = 'app-icon fallback-icon'; fallback.style.setProperty('--hue',app.id.split('').reduce((n,c) => n+c.charCodeAt(0),0)%360); fallback.textContent = [...app.name][0] || '·'; img.replaceWith(fallback); };
  const name = document.createElement('span'); name.className = 'app-name'; name.textContent = app.name;
  const more = document.createElement('button'); more.className = 'tile-menu glass-surface glass-control'; more.tabIndex = -1; more.setAttribute('aria-label',t('web.appMoreActions',{name:app.name})); more.innerHTML = icon('more');
  more.onclick = event => { event.stopPropagation(); const rect = more.getBoundingClientRect(); openMenu(app,rect.right-220,rect.bottom+6); };
  tile.append(img,name,more);
  if (state.preferences.favorites.includes(app.id)) { const star = document.createElement('span'); star.className = 'tile-favorite'; star.innerHTML = icon('star'); star.title = t('web.alreadyFavorite'); tile.append(star); }
  const edit = document.createElement('button'); edit.className = 'tile-edit glass-surface glass-control'; edit.tabIndex = -1; edit.innerHTML = icon(currentView === 'hidden' ? 'eye' : 'minus'); edit.setAttribute('aria-label',t(currentView === 'hidden' ? 'web.appRestore' : 'web.appHide',{name:app.name}));
  edit.onclick = event => { event.stopPropagation(); toggleHidden(app); }; tile.append(edit);
  tile.onclick = () => { if (Date.now()<suppressClickUntil) return; selectedID = app.id; render(); };
  tile.oncontextmenu = event => { event.preventDefault(); openMenu(app,event.clientX,event.clientY); };
  tile.onfocus = () => { selectedID = app.id; };
  tile.ondragstart = event => { draggingID = app.id; event.dataTransfer.effectAllowed = 'copyMove'; event.dataTransfer.setData('text/plain',app.id); closeMenu(); tile.classList.add('dragging'); };
  tile.ondragover = event => { if (draggingID && draggingID !== app.id) { event.preventDefault(); event.dataTransfer.dropEffect = 'move'; tile.classList.add('drop-target'); autoScroll(event.clientY); } };
  tile.ondragleave = event => { if (!tile.contains(event.relatedTarget)) tile.classList.remove('drop-target'); };
  tile.ondrop = event => { event.preventDefault(); event.stopPropagation(); if (draggingID && draggingID !== app.id) reorder(draggingID,app.id); cleanupAppDrag(); suppressClickUntil = Date.now()+350; };
  tile.ondragend = () => { cleanupAppDrag(); suppressClickUntil = Date.now()+350; };
  return tile;
}
function cleanupAppDrag() { draggingID = null; grid.querySelectorAll('.dragging,.drop-target').forEach(el => el.classList.remove('dragging','drop-target')); }
function autoScroll(y) { const scroller = $('#apps-scroll'), rect = scroller.getBoundingClientRect(); if (y<rect.top+55) scroller.scrollTop -= 14; else if (y>rect.bottom-55) scroller.scrollTop += 14; }
function reorder(source,target) {
  const order = orderedApps().map(app => app.id), from = order.indexOf(source), to = order.indexOf(target);
  if (from<0 || to<0) return;
  order.splice(from,1); order.splice(to,0,source); sortMode = 'custom'; $('#sort').value = 'custom'; selectedID = source;
  updatePreferences(preferences => { preferences.order = order; });
}
function changeView(id,{temporary = false} = {}) {
  const category = categoryByID(id); if (!category || (!category.visible && !temporary)) return;
  currentView = id; temporaryView = category.visible ? null : id; query = ''; search.value = ''; syncSearch(); selectedID = null; closeMenu(); $('#apps-scroll').scrollTop = 0; render();
}
function cycleCategory(direction,focus = false) {
  const categories = visibleCategories(); if (categories.length<2) return false;
  const index = categories.findIndex(item => item.id === currentView);
  const next = index<0 ? categories[direction>0 ? 0 : categories.length-1] : categories[(index+direction+categories.length)%categories.length];
  changeView(next.id); if (focus) tabs.focus(next.id); return true;
}
function toggleHidden(app) {
  closeMenu(); const wasHidden = state.preferences.hidden.includes(app.id);
  updatePreferences(preferences => { preferences.hidden = wasHidden ? preferences.hidden.filter(id => id !== app.id) : unique([...preferences.hidden,app.id]); });
  toast(wasHidden ? 'web.toast.appRestored' : 'web.toast.appHidden',{name:app.name},{label:'web.undo',run:() => updatePreferences(preferences => { preferences.hidden = wasHidden ? unique([...preferences.hidden,app.id]) : preferences.hidden.filter(id => id !== app.id); })});
}
function toggleFavorite(app) {
  const existing = state.preferences.favorites.includes(app.id); closeMenu();
  updatePreferences(preferences => { preferences.favorites = existing ? preferences.favorites.filter(id => id !== app.id) : unique([...preferences.favorites,app.id]); });
  toast(existing ? 'web.toast.unfavorited' : 'web.toast.favorited',{name:app.name});
}
function closeMenu() { menu.hidden = true; menuApp = null; }
function openMenu(app,x,y) {
  menuApp = app; lastAppFocus = document.activeElement;
  const hidden = state.preferences.hidden.includes(app.id), favorite = state.preferences.favorites.includes(app.id);
  menu.innerHTML = `<div class="context-menu-header"><img src="/api/icon/${encodeURIComponent(app.id)}" alt=""><strong>${escapeHTML(app.name)}</strong></div>`+
    `<button role="menuitem" data-action="launch">${icon('launch')}${htmlText('web.menu.open')}<span class="menu-key">↵</span></button><button role="menuitem" data-action="favorite">${icon('star')}${htmlText(favorite ? 'web.menu.unfavorite' : 'web.menu.favorite')}</button><button role="menuitem" data-action="categories">${icon('folder')}${htmlText('web.menu.categories')}</button>`+
    `${!builtin(currentView) ? `<button role="menuitem" data-action="remove-category">${icon('minus')}${htmlText('web.menu.removeCategory')}</button>` : ''}<div class="menu-divider" role="separator"></div><button role="menuitem" data-action="reveal">${icon('folder')}${htmlText('web.menu.reveal')}</button><button role="menuitem" data-action="copy">${icon('copy')}${htmlText('web.menu.copyPath')}</button><div class="menu-divider" role="separator"></div><button role="menuitem" class="danger" data-action="hide">${icon(hidden ? 'eye' : 'eye-off')}${htmlText(hidden ? 'web.menu.restore' : 'web.menu.hide')}</button>`;
  menu.hidden = false; const rect = menu.getBoundingClientRect();
  menu.style.left = `${Math.max(8,Math.min(x,window.innerWidth-rect.width-8))}px`; menu.style.top = `${Math.max(8,Math.min(y,window.innerHeight-rect.height-8))}px`; menu.querySelector('button').focus({preventScroll:true});
}
menu.onclick = async event => {
  const action = event.target.closest('[data-action]')?.dataset.action, app = menuApp;
  if (!action || !app) return; closeMenu();
  if (action === 'hide') return toggleHidden(app);
  if (action === 'favorite') return toggleFavorite(app);
  if (action === 'categories') return openMembershipPicker(app);
  if (action === 'remove-category') {
    const categoryID = currentView;
    return updatePreferences(preferences => { const category = preferences.categories.find(item => item.id === categoryID); if (category && !builtin(category.id)) category.appIds = category.appIds.filter(id => id !== app.id); });
  }
  try {
    if (action === 'launch') { await request('launch','POST',{id:app.id}); toast('web.toast.appOpened',{name:app.name}); }
    if (action === 'reveal') await request('reveal','POST',{id:app.id});
    if (action === 'copy') { if (isNative) await request('copy-path','POST',{id:app.id}); else await navigator.clipboard.writeText(app.path); toast('web.toast.pathCopied'); }
  } catch (err) { reportError(err); }
};
menu.onkeydown = event => {
  const buttons = [...menu.querySelectorAll('button')], index = buttons.indexOf(document.activeElement);
  if (event.key === 'ArrowDown' || event.key === 'ArrowUp') { event.preventDefault(); buttons[(index+(event.key === 'ArrowDown' ? 1 : -1)+buttons.length)%buttons.length].focus(); }
  if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); closeMenu(); lastAppFocus?.focus({preventScroll:true}); }
};
document.addEventListener('pointerdown',event => { if (!menu.hidden && !menu.contains(event.target) && !event.target.closest('.tile-menu')) closeMenu(); });
window.addEventListener('resize',closeMenu); $('#apps-scroll').addEventListener('scroll',closeMenu,{passive:true});
function syncSearch() { $('#clear-search').hidden = !search.value; }
search.oninput = () => { query = search.value.trim(); selectedID = null; syncSearch(); $('#apps-scroll').scrollTop = 0; render(); };
$('#clear-search').onclick = () => { query = ''; search.value = ''; syncSearch(); render(); search.focus(); };
$('#empty-state').onclick = event => {
  const action = event.target.closest('[data-empty-action]')?.dataset.emptyAction;
  if (action === 'clear') $('#clear-search').click();
  if (action === 'all') changeView('all',{temporary:true});
  if (action === 'add-apps') openAppPicker(currentView);
};
$('#sort').onchange = event => { sortMode = event.target.value; render(); };
for (const selector of ['#icon-size','#settings-icon-size']) $(selector).oninput = event => setSize(event.target.value,true);
async function refresh() {
  const button = $('#refresh-button'); button.disabled = true; closeMenu();
  try { if (!(await persist())) return; const fresh = await request('refresh','POST'); state.apps = fresh.apps; lastRefresh = Date.now(); render(); toast('web.toast.appsRefreshed'); }
  catch (err) { reportError(err); } finally { button.disabled = false; }
}
$('#refresh-button').onclick = refresh;
function showSettings() { closeMenu(); settings.show(); refreshTranslations(); loadUpdateSettings(); }
$('#sidebar-settings').onclick = showSettings;
$('#manage-hidden').onclick = () => { settings.close(); changeView('hidden',{temporary:true}); };
$('#settings-categories').onclick = async () => { await settings.close(); showCategories(); };
$('#reset-order').onclick = () => { sortMode = 'custom'; $('#sort').value = 'custom'; updatePreferences(preferences => { preferences.order = []; }); toast('web.toast.orderReset'); };
async function closeManager() {
  if (closingManager) return; closingManager = true; closeMenu();
  if (!isNative) { const saved = await persist(); closingManager = false; if (saved) toast('web.toast.saved'); return; }
  if (!(await prepareClose())) return;
  try { await window.mygo.window.close(); } catch (err) { cancelClosePreparation(); throw err; }
}
$('#organize-button').onclick = () => closeManager().catch(reportError);
$('#quit-button').onclick = async () => { if (!isNative) return toast('web.toast.quitInApp'); try { if (await prepareClose()) await request('quit','POST'); } catch (err) { cancelClosePreparation(); reportError(err); } };

function showCategories() { closeMenu(); renderCategoryList(); categoriesDialog.show(); }
$('#edit-categories').onclick = showCategories;
function categoryActionButton(action,label,content,disabled = false) { return `<button class="icon-button glass-surface glass-quiet" data-category-action="${action}" aria-label="${escapeHTML(label)}" title="${escapeHTML(label)}"${disabled ? ' disabled' : ''}>${icon(content)}</button>`; }
function renderCategoryList() {
  const focus = document.activeElement?.closest('[data-category-row]');
  const focusID = focus?.dataset.categoryRow, focusAction = document.activeElement?.dataset.categoryAction;
  $('#category-list').innerHTML = state.preferences.categories.map((category,index,categories) => `<div class="category-row" data-category-row="${escapeHTML(category.id)}" draggable="true"><span class="category-grip" aria-hidden="true">${icon('grip')}</span><button class="category-name-button" data-category-action="open" title="${escapeHTML(categoryName(category))}">${escapeHTML(categoryName(category))}</button><div class="category-row-actions">${categoryActionButton('up',t('web.category.moveUp',{name:categoryName(category)}),'up',index === 0)}${categoryActionButton('down',t('web.category.moveDown',{name:categoryName(category)}),'down',index === categories.length-1)}${!builtin(category.id) ? categoryActionButton('rename',t('web.category.rename',{name:categoryName(category)}),'pencil')+categoryActionButton('delete',t('web.category.delete',{name:categoryName(category)}),'trash') : '<span class="builtin-spacer"></span>'}<button class="visibility-switch" role="switch" aria-checked="${category.visible}" aria-label="${htmlText('web.category.show',{name:categoryName(category)})}" data-category-action="visibility"><span></span></button></div></div>`).join('');
  $('#new-category').disabled = state.preferences.categories.filter(item => !builtin(item.id)).length >= 64;
  if (focusID && focusAction) [...$('#category-list').querySelectorAll('[data-category-row]')].find(row => row.dataset.categoryRow === focusID)?.querySelector(`[data-category-action="${focusAction}"]`)?.focus({preventScroll:true});
}
function reorderCategory(source,target) {
  if (source === target) return;
  updatePreferences(preferences => {
    const from = preferences.categories.findIndex(item => item.id === source), to = preferences.categories.findIndex(item => item.id === target);
    if (from<0 || to<0) return;
    const [item] = preferences.categories.splice(from,1); preferences.categories.splice(to,0,item);
  });
}
$('#category-list').onclick = event => {
  const action = event.target.closest('[data-category-action]')?.dataset.categoryAction;
  const id = event.target.closest('[data-category-row]')?.dataset.categoryRow, category = categoryByID(id);
  if (!category || !action) return;
  if (action === 'open') { categoriesDialog.close(); changeView(id,{temporary:true}); }
  if (action === 'visibility') { const visible = !category.visible; updatePreferences(preferences => { const item = preferences.categories.find(item => item.id === id); if (item) item.visible = visible; }); }
  if (action === 'rename' && !builtin(id)) showNameDialog(id);
  if (action === 'up' || action === 'down') { const index = state.preferences.categories.findIndex(item => item.id === id), target = state.preferences.categories[index+(action === 'up' ? -1 : 1)]; if (target) reorderCategory(id,target.id); }
  if (action === 'delete' && !builtin(id)) {
    const deleted = clone(category), index = state.preferences.categories.findIndex(item => item.id === id);
    updatePreferences(preferences => { preferences.categories = preferences.categories.filter(item => item.id !== id); });
    toast('web.toast.categoryDeleted',{name:categoryName(category)},{label:'web.undo',run:() => updatePreferences(preferences => { if (!preferences.categories.some(item => item.id === id)) preferences.categories.splice(Math.min(index,preferences.categories.length),0,deleted); })});
  }
};
$('#category-list').ondragstart = event => { const row = event.target.closest('[data-category-row]'); if (!row) return; draggedCategoryID = row.dataset.categoryRow; event.dataTransfer.effectAllowed = 'move'; event.dataTransfer.setData('application/x-gopad-category',draggedCategoryID); row.classList.add('dragging'); };
$('#category-list').ondragover = event => { const row = event.target.closest('[data-category-row]'); if (!draggedCategoryID || !row) return; event.preventDefault(); event.dataTransfer.dropEffect = 'move'; row.classList.add('drop-target'); };
$('#category-list').ondragleave = event => { const row = event.target.closest('[data-category-row]'); if (row && !row.contains(event.relatedTarget)) row.classList.remove('drop-target'); };
$('#category-list').ondrop = event => { const row = event.target.closest('[data-category-row]'); if (!row || !draggedCategoryID) return; event.preventDefault(); reorderCategory(draggedCategoryID,row.dataset.categoryRow); cleanupCategoryDrag(); };
$('#category-list').ondragend = cleanupCategoryDrag;
function cleanupCategoryDrag() { draggedCategoryID = null; $('#category-list').querySelectorAll('.dragging,.drop-target').forEach(row => row.classList.remove('dragging','drop-target')); }
function randomCategoryID() {
  if (crypto.randomUUID) return `custom-${crypto.randomUUID()}`;
  const bytes = crypto.getRandomValues(new Uint8Array(16)); bytes[6] = (bytes[6]&15)|64; bytes[8] = (bytes[8]&63)|128;
  const hex = [...bytes].map(n => n.toString(16).padStart(2,'0')).join('');
  return `custom-${hex.slice(0,8)}-${hex.slice(8,12)}-${hex.slice(12,16)}-${hex.slice(16,20)}-${hex.slice(20)}`;
}
function showNameDialog(id = null) {
  nameSession++;
  editingCategoryID = id;
  $('#save-category-name').disabled = false;
  renderNameLabels();
  $('#category-name').value = id ? categoryByID(id).name : ''; $('#category-name-error').hidden = true;
  nameDialog.show().then(() => { $('#category-name').focus(); $('#category-name').select(); });
}
$('#new-category').onclick = () => showNameDialog();
$('#category-name-form').onsubmit = async event => {
  event.preventDefault();
  if (isComposing(event)) return;
  const name = $('#category-name').value.trim(), error = $('#category-name-error'), button = $('#save-category-name');
  if (!name || Array.from(name).length>40) { localizedText(error,name ? 'web.categoryNameTooLong' : 'web.enterCategoryName',{max:40}); error.hidden = false; return; }
  const id = editingCategoryID || randomCategoryID(), creating = !editingCategoryID;
  const session = nameSession, parentPickerSession = pickerSession;
  if (creating && state.preferences.categories.filter(item => !builtin(item.id)).length>=64) { localizedText(error,'web.categoryLimit',{max:64}); error.hidden = false; return; }
  button.disabled = true; error.hidden = true;
  const ok = await updatePreferences(preferences => {
    const category = preferences.categories.find(item => item.id === id);
    if (creating && !category) preferences.categories.push({id,name,appIds:[],visible:true});
    else if (category && !builtin(id)) category.name = name;
  });
  if (session !== nameSession || !nameDialog.open || nameDialog.element.classList.contains('closing')) return;
  button.disabled = false;
  if (!ok) { localizedText(error,'web.saveRetry'); error.hidden = false; return; }
  if (creating && picker.open && parentPickerSession === pickerSession && pickerState?.mode === 'memberships') { pickerState.selection.add(id); renderPicker(); }
  await nameDialog.close();
};
function openMembershipPicker(app) {
  pickerState = {mode:'memberships',appID:app.id,selection:new Set(state.preferences.categories.filter(category => !builtin(category.id) && category.appIds.includes(app.id)).map(category => category.id))};
  openPicker();
}
function openAppPicker(categoryID) {
  const category = categoryByID(categoryID); if (!category || builtin(categoryID)) return;
  pickerState = {mode:'apps',categoryID,selection:new Set(category.appIds)}; openPicker();
}
function openPicker() { pickerSession++; $('#save-picker').disabled = false; $('#picker-search').value = ''; $('#picker-error').hidden = true; renderPicker(); picker.show(); }
function renderPicker() {
  if (!pickerState) return;
  const mode = pickerState.mode, selection = pickerState.selection, term = normalize($('#picker-search').value);
  localizedText($('#picker-title'),mode === 'memberships' ? 'web.addToCategories' : 'web.addApps');
  $('#picker-context').textContent = mode === 'memberships' ? appByID(pickerState.appID)?.name || '' : categoryName(categoryByID(pickerState.categoryID));
  $('#picker-new-category').hidden = mode !== 'memberships';
  const options = mode === 'memberships' ? state.preferences.categories.filter(category => !builtin(category.id)).map(category => ({id:category.id,name:category.name,hidden:!category.visible})) : orderedApps().filter(app => !state.preferences.hidden.includes(app.id) || selection.has(app.id)).map(app => ({id:app.id,name:app.name,app:true,hidden:state.preferences.hidden.includes(app.id)}));
  const shown = options.filter(option => !term || normalize(option.name).includes(term));
  const focused = document.activeElement?.dataset.option;
  $('#picker-list').innerHTML = shown.map(option => `<label class="picker-option"><input type="checkbox" data-option="${escapeHTML(option.id)}"${selection.has(option.id) ? ' checked' : ''}/><span class="picker-option-check">${icon('check')}</span>${option.app ? `<img src="/api/icon/${encodeURIComponent(option.id)}" alt="" loading="lazy"/>` : `<span class="picker-option-icon">${icon('folder')}</span>`}<span class="picker-option-name">${escapeHTML(option.name)}</span>${option.hidden ? `<span class="picker-option-hidden" title="${htmlText('web.hiddenLabel')}">${icon('eye-off')}</span>` : ''}</label>`).join('') || `<div class="picker-empty">${htmlText(term ? 'web.noResults' : mode === 'memberships' ? 'web.noCustomCategories' : 'web.noApps')}</div>`;
  localizedText($('#picker-selection-count'),'web.selectionCount',{count:selection.size});
  if (focused) [...$('#picker-list').querySelectorAll('[data-option]')].find(input => input.dataset.option === focused)?.focus({preventScroll:true});
}
$('#picker-search').oninput = renderPicker;
$('#picker-list').onchange = event => { const id = event.target.dataset.option; if (!id || !pickerState) return; if (event.target.checked) pickerState.selection.add(id); else pickerState.selection.delete(id); localizedText($('#picker-selection-count'),'web.selectionCount',{count:pickerState.selection.size}); };
$('#picker-new-category').onclick = () => showNameDialog();
$('#save-picker').onclick = async () => {
  if (!pickerState) return;
  const draft = {...pickerState,selection:new Set(pickerState.selection)}, button = $('#save-picker'), error = $('#picker-error'), session = pickerSession;
  if (draft.mode === 'apps' && !categoryByID(draft.categoryID)) { localizedText(error,'web.categoryDeleted'); error.hidden = false; return; }
  button.disabled = true; error.hidden = true;
  const ok = await updatePreferences(preferences => {
    if (draft.mode === 'apps') { const category = preferences.categories.find(item => item.id === draft.categoryID); if (category) category.appIds = [...draft.selection]; }
    else for (const category of preferences.categories) if (!builtin(category.id)) category.appIds = draft.selection.has(category.id) ? unique([...category.appIds,draft.appID]) : category.appIds.filter(id => id !== draft.appID);
  });
  if (session !== pickerSession || !picker.open || picker.element.classList.contains('closing')) return;
  button.disabled = false;
  if (!ok) { localizedText(error,'web.saveRetry'); error.hidden = false; return; }
  await picker.close();
};
function renderNameLabels() {
  localizedText($('#category-name-title'),editingCategoryID ? 'web.renameCategory' : 'web.newCategory');
  localizedText($('#save-category-name'),editingCategoryID ? 'web.save' : 'web.create');
}
function renderLanguageSelector() {
  const selector = $('#language'), preference = state.preferences.language;
  let unknown = selector.querySelector('[data-language-unknown]');
  if (unknown && unknown.value !== preference) { unknown.remove(); unknown = null; }
  if (!['system','en','zh-CN'].includes(preference)) {
    if (!unknown) { unknown = document.createElement('option'); unknown.dataset.languageUnknown = ''; unknown.value = preference; selector.append(unknown); }
    localizedText(unknown,'web.language.unavailable',{language:preference});
  }
  selector.value = preference;
}
$('#language').onchange = event => {
  const language = event.target.value;
  updatePreferences(preferences => { preferences.language = language; });
};
async function refreshTranslations({report = false} = {}) {
  try { await window.GopadI18n.load(); }
  catch (err) { console.warn(err); if (report) toast('web.language.loadError'); }
}
document.addEventListener('gopad:languagechange',() => {
  translationRevision++;
  if (loading) return;
  const openApp = !menu.hidden ? menuApp : null;
  const menuPosition = {x:parseFloat(menu.style.left)||8,y:parseFloat(menu.style.top)||8};
  const menuAction = document.activeElement?.dataset.action;
  render(); renderNameLabels(); renderUpdateSettings();
  if (openApp) { openMenu(openApp,menuPosition.x,menuPosition.y); if (menuAction) menu.querySelector(`[data-action="${menuAction}"]`)?.focus({preventScroll:true}); }
  if (!$('#toast').hidden) renderToast();
});
window.addEventListener('mygo:locale',refreshTranslations);

let updateSettings = {status:'unavailable',currentVersion:'—',automaticChecks:true};
let updateSettingsLoaded = false, updateAction = null, updateGeneration = 0;
const updateStatuses = new Set(['ready','development','not_configured','unavailable','preview']);
function acceptUpdateSettings(data) {
  updateSettings = {status:updateStatuses.has(data.status) ? data.status : 'unavailable',currentVersion:typeof data.currentVersion === 'string' && data.currentVersion ? data.currentVersion : window.GopadI18n.metadata.version || '—',automaticChecks:data.automaticChecks === true};
  updateSettingsLoaded = true;
}
function renderUpdateSettings() {
  $('#current-version').textContent = updateSettingsLoaded ? updateSettings.currentVersion : window.GopadI18n.metadata.version || '—';
  const status = $('#update-status'), ready = updateSettings.status === 'ready';
  status.hidden = !updateSettingsLoaded || ready;
  if (!status.hidden) localizedText(status,`web.update.status.${updateSettings.status}`);
  const automatic = $('#automatic-updates');
  automatic.setAttribute('aria-checked',String(updateSettings.automaticChecks));
  automatic.disabled = !updateSettingsLoaded || !ready || !!updateAction;
  const check = $('#check-updates');
  check.disabled = !updateSettingsLoaded || !ready || !!updateAction;
  check.setAttribute('aria-busy',String(updateAction === 'check'));
  localizedText(check,updateAction === 'check' ? 'web.update.opening' : 'web.update.check');
}
async function loadUpdateSettings() {
  if (updateAction) { renderUpdateSettings(); return; }
  const generation = ++updateGeneration;
  updateAction = 'load'; renderUpdateSettings();
  try { const data = await request('update'); if (generation === updateGeneration) acceptUpdateSettings(data); }
  catch (err) { if (generation === updateGeneration) { updateSettings = {...updateSettings,status:'unavailable'}; updateSettingsLoaded = true; } console.warn(err); }
  finally { if (generation === updateGeneration) { updateAction = null; renderUpdateSettings(); } }
}
$('#check-updates').onclick = async () => {
  if (updateAction || updateSettings.status !== 'ready') return;
  updateAction = 'check'; renderUpdateSettings();
  try { if (await persist()) await request('update/check','POST'); }
  catch (err) { reportError(err); }
  finally { updateAction = null; renderUpdateSettings(); }
};
$('#automatic-updates').onclick = async () => {
  if (updateAction || updateSettings.status !== 'ready') return;
  const previous = {...updateSettings}, automaticChecks = !updateSettings.automaticChecks;
  const generation = ++updateGeneration;
  updateAction = 'preferences'; updateSettings = {...updateSettings,automaticChecks}; renderUpdateSettings();
  try { const data = await request('update/preferences','PUT',{automaticChecks}); if (generation === updateGeneration) acceptUpdateSettings(data); }
  catch (err) { if (generation === updateGeneration) updateSettings = previous; reportError(err,'web.saveError'); }
  finally { if (generation === updateGeneration) { updateAction = null; renderUpdateSettings(); } }
};

function moveSelection(key) {
  const tiles = [...grid.querySelectorAll('.app-tile')]; if (!tiles.length) return;
  const cols = getComputedStyle(grid).gridTemplateColumns.split(' ').length;
  let index = tiles.findIndex(tile => tile.dataset.id === selectedID);
  index = index<0 ? 0 : Math.max(0,Math.min(tiles.length-1,index+({ArrowLeft:-1,ArrowRight:1,ArrowUp:-cols,ArrowDown:cols}[key]||0)));
  selectedID = tiles[index].dataset.id; tiles.forEach(tile => tile.classList.toggle('keyboard-selected',tile.dataset.id === selectedID)); tiles[index].focus({preventScroll:true}); tiles[index].scrollIntoView?.({block:'nearest'});
}
document.addEventListener('keydown',event => {
  if (event.defaultPrevented || isComposing(event)) return;
  const dialog = GlassDialog.top();
  if (event.key === 'Escape') { event.preventDefault(); if (!menu.hidden) closeMenu(); else if (dialog) dialog.close(); else closeManager().catch(reportError); return; }
  if (dialog || !menu.hidden) return;
  if (event.key === 'Tab' && !event.metaKey && !event.altKey) {
    const target = event.target, tab = target.closest?.('[role="tab"]');
    const ordinaryControl = target.closest?.('input,textarea,select,button,a') && target !== search && !tab;
    if (event.ctrlKey || !ordinaryControl) {
      if (cycleCategory(event.shiftKey ? -1 : 1,!!tab || target !== search)) event.preventDefault();
    }
    return;
  }
  if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'f') { event.preventDefault(); search.focus(); search.select(); return; }
  if ((event.metaKey || event.ctrlKey) && event.key === ',') { event.preventDefault(); showSettings(); return; }
  if ((event.metaKey || event.ctrlKey) && /^[1-9]$/.test(event.key)) { const category = visibleCategories()[Number(event.key)-1]; if (category) { event.preventDefault(); changeView(category.id); } return; }
  if (['ArrowUp','ArrowDown','ArrowLeft','ArrowRight'].includes(event.key) && !['SELECT','BUTTON','TEXTAREA'].includes(event.target.tagName) && (event.target.tagName !== 'INPUT' || (event.target === search && event.key === 'ArrowDown'))) { event.preventDefault(); moveSelection(event.key); return; }
  if (event.key === 'Enter' && (event.target === search || event.target.closest('.app-tile') === event.target)) {
    const app = appByID(selectedID) || visibleApps()[0], tile = [...grid.children].find(el => el.dataset.id === app?.id), rect = tile?.getBoundingClientRect();
    if (rect) { event.preventDefault(); openMenu(app,rect.left+rect.width/2,rect.top+rect.height/2); } return;
  }
  if ((event.key === 'ContextMenu' || (event.shiftKey && event.key === 'F10')) && selectedID) { event.preventDefault(); const tile = [...grid.children].find(el => el.dataset.id === selectedID), rect = tile?.getBoundingClientRect(); if (rect) openMenu(appByID(selectedID),rect.left+rect.width/2,rect.top+rect.height/2); }
  if (!event.metaKey && !event.ctrlKey && !event.altKey && event.key.length === 1 && !['INPUT','SELECT','TEXTAREA'].includes(event.target.tagName) && !event.target.closest('button')) search.focus();
});
async function initialize() {
  loading = true;
  grid.innerHTML = Array.from({length:21},() => '<div class="app-tile skeleton"><div class="skeleton-icon"></div><div class="skeleton-label"></div></div>').join('');
  try {
    await window.GopadI18n.load();
    const initial = await request('state'); state.apps = initial.apps; lastConfirmedPreferences = normalizePreferences(initial.preferences);
    if (lastConfirmedPreferences.categories.some(category => category.id === lastConfirmedPreferences.lastCategory && category.visible)) currentView = lastConfirmedPreferences.lastCategory;
    loading = false; replayPending();
  } catch (err) {
    loading = false; grid.replaceChildren(); localizedText($('#collection-count'),'web.loadingFailed'); $('#empty-state').hidden = false;
    $('#empty-state').innerHTML = `<div class="empty-illustration">${icon('info')}</div><h2>${htmlText('web.cannotLoadApps')}</h2><p>${escapeHTML(err.message)}</p><button class="button glass-surface glass-control" id="retry-load">${htmlText('web.retryLoad')}</button>`; $('#retry-load').onclick = initialize;
  }
}
window.addEventListener('focus',async () => {
  if (!loading) refreshTranslations();
  if (settings.open) loadUpdateSettings();
  if (loading || Date.now()-lastRefresh<30000) return; lastRefresh = Date.now();
  try { if (!(await persist())) return; const fresh = await request('refresh','POST'); state.apps = fresh.apps; render(); } catch (err) { console.warn(err); }
});
initialize();
