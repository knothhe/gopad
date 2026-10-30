/* Small shared controls; no build step or runtime dependency. */
(() => {
  const reducedMotion = () => window.matchMedia?.('(prefers-reduced-motion: reduce)').matches || false;
  const duration = () => reducedMotion() ? 0 : 180;
  const stack = [];
  let composing = false;
  document.addEventListener('compositionstart', () => { composing = true; });
  document.addEventListener('compositionend', () => { composing = false; });
  const isComposing = event => composing || event.isComposing || event.keyCode === 229;
  class GlassDialog {
    constructor(element) {
      this.element = element;
      this.pendingClose = null;
      element.addEventListener('cancel', event => { event.preventDefault(); this.close(); });
      element.addEventListener('click', event => {
        if (event.target !== element) return;
        const r = element.getBoundingClientRect();
        if (event.clientX < r.left || event.clientX > r.right || event.clientY < r.top || event.clientY > r.bottom) this.close();
      });
      element.querySelectorAll('[data-dialog-close]').forEach(button => button.addEventListener('click', () => this.close()));
      element.addEventListener('close', () => {
        const index = stack.indexOf(this);
        if (index >= 0) stack.splice(index, 1);
      });
    }
    get open() { return this.element.open; }
    show() {
      if (this.pendingClose) return this.pendingClose.then(() => this.show());
      if (!this.open) { this.element.classList.remove('closing'); this.element.showModal(); stack.push(this); }
      return Promise.resolve();
    }
    close() {
      if (this.pendingClose) return this.pendingClose;
      if (!this.open) return Promise.resolve();
      this.element.classList.add('closing');
      this.pendingClose = new Promise(resolve => setTimeout(() => {
        this.element.close();
        const index = stack.indexOf(this);
        if (index >= 0) stack.splice(index, 1);
        this.element.classList.remove('closing');
        this.pendingClose = null;
        resolve();
      }, duration()));
      return this.pendingClose;
    }
    static top() { return [...stack].reverse().find(dialog => dialog.open); }
    static async closeAll() { for (const dialog of [...stack].reverse()) await dialog.close(); }
  }
  class CategoryTabs {
    constructor(element, callbacks) {
      this.element = element;
      this.callbacks = callbacks;
      this.items = [];
      this.draggedCategory = null;
      element.addEventListener('click', event => {
        const tab = event.target.closest('[role="tab"]');
        if (tab) callbacks.select(tab.dataset.view);
      });
      element.addEventListener('keydown', event => {
        const tab = event.target.closest('[role="tab"]');
        if (isComposing(event)) return;
        if (!tab || !['ArrowLeft','ArrowRight','Home','End'].includes(event.key)) return;
        event.preventDefault();
        const index = this.items.findIndex(item => item.id === tab.dataset.view);
        const next = event.key === 'Home' ? 0 : event.key === 'End' ? this.items.length - 1 : (index + (event.key === 'ArrowRight' ? 1 : -1) + this.items.length) % this.items.length;
        callbacks.select(this.items[next].id);
        this.focus(this.items[next].id);
      });
      element.addEventListener('dragstart', event => {
        const tab = event.target.closest('[role="tab"]');
        if (!tab) return;
        this.draggedCategory = tab.dataset.view;
        event.dataTransfer.effectAllowed = 'move';
        event.dataTransfer.setData('application/x-gopad-category', this.draggedCategory);
        tab.classList.add('dragging');
      });
      element.addEventListener('dragover', event => {
        const tab = event.target.closest('[role="tab"]');
        if (!tab || (!this.draggedCategory && !callbacks.canDropApp(tab.dataset.view))) return;
        event.preventDefault();
        event.dataTransfer.dropEffect = this.draggedCategory ? 'move' : 'copy';
        tab.classList.add('drop-target');
      });
      element.addEventListener('dragleave', event => {
        const tab = event.target.closest('[role="tab"]');
        if (tab && !tab.contains(event.relatedTarget)) tab.classList.remove('drop-target');
      });
      element.addEventListener('drop', event => {
        const tab = event.target.closest('[role="tab"]');
        if (!tab) return;
        event.preventDefault();
        if (this.draggedCategory) callbacks.reorder(this.draggedCategory, tab.dataset.view);
        else callbacks.dropApp(tab.dataset.view);
        this.cleanupDrag();
      });
      element.addEventListener('dragend', () => this.cleanupDrag());
    }
    cleanupDrag() { this.draggedCategory = null; this.element.querySelectorAll('.dragging,.drop-target').forEach(el => el.classList.remove('dragging','drop-target')); }
    focus(id) { this.element.querySelectorAll('[role="tab"]').forEach(tab => { if (tab.dataset.view === id) tab.focus({preventScroll:true}); }); }
    render(items, current, count) {
      this.items = items;
      const focused = document.activeElement?.closest('[role="tab"]')?.dataset.view;
      this.element.replaceChildren(...items.map(item => {
        const button = document.createElement('button');
        button.className = 'nav-item' + (item.id === current ? ' active' : '') + (!item.visible ? ' temporary' : '');
        button.id = `category-tab-${item.id}`;
        button.dataset.view = item.id;
        button.setAttribute('role','tab');
        button.draggable = true;
        button.tabIndex = item.id === current ? 0 : -1;
        button.setAttribute('aria-selected', String(item.id === current));
        button.setAttribute('aria-controls', 'apps-scroll');
        button.title = item.visible ? item.name : window.GopadI18n.t('web.category.temporary',{name:item.name});
        const name = document.createElement('span'); name.className = 'tab-name'; name.textContent = item.name;
        const total = document.createElement('span'); total.className = 'nav-count'; total.textContent = count(item); total.setAttribute('aria-hidden','true');
        if (['all','favorites','hidden'].includes(item.id)) total.id = `${item.id}-count`;
        button.append(name,total);
        return button;
      }));
      if (focused && items.some(item => item.id === focused)) this.focus(focused);
      const active = this.element.querySelector('[aria-selected="true"]');
      active?.scrollIntoView?.({block:'nearest',inline:'nearest',behavior:'auto'});
    }
  }
  // The toolbar follows icon frames, including the flexible space inside each
  // auto-fill grid cell. This uses the same minimum width and gap as the grid.
  function gridInlineInset(width,minimumCellWidth,iconSize,columnGap) {
    if (!Number.isFinite(width) || width <= 0) return 0;
    const minimum = Math.max(1,minimumCellWidth), gap = Math.max(0,columnGap);
    const columns = Math.max(1,Math.floor((width+gap)/(minimum+gap)));
    const cell = (width-gap*(columns-1))/columns;
    return Math.max(0,(cell-iconSize)/2);
  }
  window.GopadUI = {GlassDialog,CategoryTabs,reducedMotion,duration,isComposing,gridInlineInset};
})();
