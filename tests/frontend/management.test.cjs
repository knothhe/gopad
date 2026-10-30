const {test} = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const {JSDOM} = require('jsdom');
const root = path.resolve(__dirname,'../..');
const apps = [
  {id:'gamma',name:'Gamma',path:'/Applications/Gamma.app',kind:'application'},
  {id:'alpha',name:'Alpha',path:'/Applications/Alpha.app',kind:'application'},
  {id:'beta',name:'Beta',path:'/System/Applications/Beta.app',kind:'system'},
  {id:'delta',name:'Delta',path:'/Applications/Delta.app',kind:'application'},
];
const builtins = () => ['all','favorites','system','hidden'].map((id,index) => ({id,name:['全部','收藏','系统','隐藏'][index],appIds:[],visible:id !== 'hidden'}));
const custom = (id,name,appIds=[],visible=true) => ({id,name,appIds,visible});
const tick = (ms=12) => new Promise(resolve => setTimeout(resolve,ms));
const deferred = () => { let resolve; const promise = new Promise(done => {resolve=done}); return {promise,resolve}; };
async function fixture(t,options={}) {
  const dom = new JSDOM(fs.readFileSync(path.join(root,'frontend/index.html'),'utf8'),{url:'http://127.0.0.1:5192/',runScripts:'outside-only',pretendToBeVisual:true});
  const w = dom.window;
  const control = {systemLocale:options.systemLocale || 'zh-CN',metadata:{appName:'gopad Dev',shortcut:'⌃ ⌥ ⇧ Space',version:'dev',channel:'dev',...options.metadata},updateState:{status:'ready',currentVersion:'0.1.0',automaticChecks:true,...options.updateState},updatePlans:[],i18nPlans:[],actionFailures:{},warnings:[],preferences:{hidden:['delta'],favorites:[],order:[],iconSize:80,language:'system',lastCategory:'all',...structuredClone(options.preferences||{})},plans:[],saves:[],actions:[],closed:0,errors:[]};
  w.structuredClone = structuredClone;
  w.matchMedia = () => ({matches:true});
  w.mygo = {window:{close:async()=>{control.closed++}}};
  w.console.error = error => {control.errors.push(error)};
  w.console.warn = error => {control.warnings.push(error)};
  w.Element.prototype.scrollIntoView = function(){};
  w.HTMLDialogElement.prototype.showModal = function(){this.open=true};
  w.HTMLDialogElement.prototype.close = function(){this.open=false;this.dispatchEvent(new w.Event('close'))};
  w.fetch = async (url,request={}) => {
    control.actions.push({url,method:request.method});
    if(url==='/api/i18n') {
      const plan=await control.i18nPlans.shift()?.();
      if(plan?.status) return Response.json({error:'I18N_MESSAGES_UNAVAILABLE'},{status:plan.status});
      const language=control.preferences.language || 'system';
      const effective=language==='system' ? control.systemLocale : language;
      const locale=/^zh(?:-CN|-SG|-Hans)?$/i.test(effective) ? 'zh-CN' : 'en';
      const directory=path.join(root,'internal/i18n/locales',locale);
      const messages=Object.assign({},...fs.readdirSync(directory).filter(name=>name.endsWith('.json')).map(name=>JSON.parse(fs.readFileSync(path.join(directory,name),'utf8'))));
      return Response.json({locale,language,messages,...control.metadata});
    }
    if(url==='/api/update') return Response.json(control.updateState);
    if(url==='/api/update/check') return Response.json({ok:true});
    if(url==='/api/update/preferences') {
      const plan=await control.updatePlans.shift()?.(JSON.parse(request.body));
      if(plan?.status) return Response.json({error:'update preference failed',errorKey:'error.updateSave'},{status:plan.status});
      control.updateState={...control.updateState,...JSON.parse(request.body)};
      return Response.json(control.updateState);
    }
    if(control.actionFailures[url]) return Response.json(control.actionFailures[url],{status:500});
    if(url==='/api/preferences') {
      const candidate=JSON.parse(request.body);control.saves.push(candidate);
      const result=await control.plans.shift()?.(structuredClone(candidate));
      if(result?.status) return Response.json({error:result.error||'测试保存失败',errorKey:result.errorKey},{status:result.status});
      control.preferences=structuredClone(result?.preferences||candidate);
    }
    return Response.json(['/api/state','/api/preferences','/api/refresh'].includes(url)?{apps,preferences:structuredClone(control.preferences)}:{ok:true});
  };
  w.eval(fs.readFileSync(path.join(root,'frontend/components/i18n.js'),'utf8'));
  w.eval(fs.readFileSync(path.join(root,'frontend/components/ui.js'),'utf8'));
  w.eval(fs.readFileSync(path.join(root,'frontend/app.js'),'utf8'));
  t.after(()=>dom.window.close());
  const $ = selector => w.document.querySelector(selector);
  const event = (type,data={}) => {const event=new w.Event(type,{bubbles:true,cancelable:true});Object.assign(event,data);return event;};
  const key = (target,key,data={}) => {const event=new w.KeyboardEvent('keydown',{key,bubbles:true,cancelable:true,...data});target.dispatchEvent(event);return event;};
  const input = (selector,value) => {$(selector).value=value;$(selector).dispatchEvent(event('input'));};
  const checked = (id,value) => {const el=$(`#picker-list [data-option="${id}"]`);assert.ok(el,`missing picker ${id}`);el.checked=value;el.dispatchEvent(event('change'));};
  const ids = () => [...w.document.querySelectorAll('.app-tile')].map(tile=>tile.dataset.id);
  const active = () => $('#navigation [aria-selected="true"]')?.dataset.view;
  const submitName = name => {$('#category-name').value=name;$('#category-name-form').dispatchEvent(event('submit'));};
  await tick();
  return {w,$,control,event,key,input,checked,ids,active,submitName};
}

test('legacy migration, stable API order, visible tab cycle and accessible roving navigation',async t=>{
  const {w,$,control,key,input,ids,active}=await fixture(t);
  assert.deepEqual(ids(),['gamma','alpha','beta']);
  assert.deepEqual([...$('#navigation').children].map(el=>el.dataset.view),['all','favorites','system']);
  assert.equal($('[data-view="hidden"]'),null);
  input('#search','Gamma');key($('#search'),'Tab');assert.equal(active(),'favorites');assert.equal($('#search').value,'');assert.deepEqual(ids(),[]);
  key($('#search'),'Tab');assert.equal(active(),'system');key($('#search'),'Tab');assert.equal(active(),'all');
  key($('#search'),'Tab',{shiftKey:true});assert.equal(active(),'system');
  key($('[data-view="system"]'),'ArrowRight');assert.equal(active(),'all');
  assert.equal($('#navigation [aria-selected="true"]').tabIndex,0);
  assert.equal([...$('#navigation').children].filter(el=>el.tabIndex===0).length,1);
  assert.equal(key($('#icon-size'),'Tab').defaultPrevented,false);
  $('#sidebar-settings').click();assert.equal(key($('#settings-icon-size'),'Tab').defaultPrevented,false);
  key(w.document,'Escape');await tick();assert.equal($('#settings-dialog').open,false);assert.equal(control.closed,0);
  assert.equal($('[data-view="hidden"]'),null);
});

test('IME composition keeps Enter, Escape and Tab inside the input method',async t=>{
  const {w,$,control,key,active}=await fixture(t);
  $('#search').dispatchEvent(new w.CompositionEvent('compositionstart',{bubbles:true}));
  for(const value of ['Tab','Enter','Escape']) assert.equal(key($('#search'),value).defaultPrevented,false);
  assert.equal(active(),'all');assert.equal($('#context-menu').hidden,true);assert.equal(control.closed,0);
  $('#search').dispatchEvent(new w.CompositionEvent('compositionend',{bubbles:true}));
  assert.equal(key($('#search'),'Tab',{isComposing:true}).defaultPrevented,false);
  key($('#search'),'Tab');assert.equal(active(),'favorites');
});

test('drag order, hide undo, temporary hidden management, favorite and size rollback remain usable',async t=>{
  const {w,$,control,event,input,ids}=await fixture(t);
  const dataTransfer={effectAllowed:'',dropEffect:'',setData(){}};
  $('[data-id="gamma"]').dispatchEvent(event('dragstart',{dataTransfer}));$('[data-id="beta"]').dispatchEvent(event('drop',{dataTransfer}));await tick();
  assert.deepEqual(ids(),['alpha','beta','gamma']);assert.deepEqual(control.preferences.order,['alpha','beta','gamma','delta']);
  $('[aria-label="隐藏 Gamma"]').click();await tick();assert.deepEqual(ids(),['alpha','beta']);
  $('#toast button').click();await tick();assert.deepEqual(ids(),['alpha','beta','gamma']);
  $('#sidebar-settings').click();$('#manage-hidden').click();await tick();assert.deepEqual(ids(),['delta']);
  assert.equal(control.preferences.categories.find(c=>c.id==='hidden').visible,false);
  $('[aria-label="恢复显示 Delta"]').click();await tick();assert.deepEqual(ids(),[]);
  $('[data-view="all"]').click();assert.equal($('[data-view="hidden"]'),null);assert.deepEqual(ids(),['alpha','beta','gamma','delta']);
  $('[aria-label="Gamma 的更多操作"]').click();$('#context-menu [data-action="favorite"]').click();await tick();
  $('[data-view="favorites"]').click();assert.deepEqual(ids(),['gamma']);
  input('#icon-size','112');await tick();assert.equal(control.preferences.iconSize,112);
  control.plans.push(()=>({status:500}));input('#icon-size','104');await tick();assert.equal($('#icon-size').value,'112');assert.equal(control.preferences.iconSize,112);
  input('#icon-size','96');await tick();assert.equal(control.preferences.iconSize,96);
  input('#search','<script>');assert.ok($('#empty-state').textContent.includes('<script>'));assert.equal($('#empty-state script'),null);
  assert.equal(w.document.querySelector('.sidebar'),null);
});

test('create, Unicode rename, keyboard and drag category ordering, visibility, delete and undo',async t=>{
  const {w,$,control,event,submitName,input,active}=await fixture(t,{preferences:{categories:builtins()}});
  $('#edit-categories').click();$('#new-category').click();submitName('工作');await tick();
  let category=control.preferences.categories.find(c=>c.name==='工作');assert.ok(category.id.startsWith('custom-'));assert.equal(category.visible,true);
  const id=category.id;
  $(`[data-category-row="${id}"] [data-category-action="rename"]`).click();submitName('🌙'.repeat(41));await tick();assert.equal($('#category-name-error').hidden,false);assert.equal($('#category-name-dialog').open,true);
  submitName('🌙'.repeat(40));await tick();assert.equal(control.preferences.categories.find(c=>c.id===id).name,'🌙'.repeat(40));
  $(`[data-category-row="${id}"] [data-category-action="up"]`).click();await tick();assert.equal(control.preferences.categories.at(-2).id,id);
  const dataTransfer={effectAllowed:'',dropEffect:'',setData(){}};
  $(`[data-category-row="${id}"]`).dispatchEvent(event('dragstart',{dataTransfer}));$('[data-category-row="all"]').dispatchEvent(event('drop',{dataTransfer}));await tick();assert.equal(control.preferences.categories[0].id,id);
  $(`[data-category-row="${id}"] [data-category-action="open"]`).click();await tick();assert.equal(active(),id);
  input('#search','absent');$('#apps-scroll').scrollTop=100;
  $('#edit-categories').click();$(`[data-category-row="${id}"] [data-category-action="visibility"]`).click();await tick();
  assert.equal(active(),'all');assert.equal($('#search').value,'');assert.equal($('#apps-scroll').scrollTop,0);assert.equal($(`[data-view="${id}"]`),null);
  assert.equal($('[data-category-row="all"] [data-category-action="delete"]'),null);
  $(`[data-category-row="${id}"] [data-category-action="delete"]`).click();await tick();assert.equal(control.preferences.categories.some(c=>c.id===id),false);
  $('#toast button').click();await tick();assert.ok(control.preferences.categories.some(c=>c.id===id));
  for(const category of [...control.preferences.categories]) if(category.visible) $(`[data-category-row="${category.id}"] [data-category-action="visibility"]`).click();
  await tick(35);assert.ok(control.preferences.categories.some(c=>c.visible));assert.ok(control.preferences.categories.find(c=>c.id==='all').visible);
});

test('multi-select membership, app picker, drag to a tab and removing one membership preserve other sections',async t=>{
  const categories=[...builtins(),custom('work','工作',['alpha']),custom('custom-play','娱乐',[],false),custom('custom-other','其他',['gamma'])];
  const {w,$,control,event,checked,ids}=await fixture(t,{preferences:{categories}});
  assert.ok($('[data-view="work"]'),'valid server custom ID without custom- prefix survives');
  $('[aria-label="Gamma 的更多操作"]').click();$('#context-menu [data-action="categories"]').click();
  checked('work',true);checked('custom-play',true);checked('custom-other',false);$('#save-picker').click();await tick();
  assert.deepEqual(control.preferences.categories.find(c=>c.id==='work').appIds,['alpha','gamma']);
  assert.deepEqual(control.preferences.categories.find(c=>c.id==='custom-play').appIds,['gamma']);
  $('[data-view="work"]').click();assert.deepEqual(ids(),['gamma','alpha']);
  $('[aria-label="Gamma 的更多操作"]').click();$('#context-menu [data-action="remove-category"]').click();await tick();assert.deepEqual(ids(),['alpha']);assert.deepEqual(control.preferences.categories.find(c=>c.id==='custom-play').appIds,['gamma']);
  $('[data-view="custom-other"]').click();$('#empty-state [data-empty-action="add-apps"]').click();checked('beta',true);$('#save-picker').click();await tick();assert.deepEqual(ids(),['beta']);
  $('[data-view="all"]').click();const dataTransfer={effectAllowed:'',dropEffect:'',setData(){}};
  $('[data-id="alpha"]').dispatchEvent(event('dragstart',{dataTransfer}));$('[data-view="custom-other"]').dispatchEvent(event('drop',{dataTransfer}));await tick();
  assert.deepEqual(control.preferences.categories.find(c=>c.id==='custom-other').appIds,['beta','alpha']);assert.deepEqual(control.preferences.categories.find(c=>c.id==='work').appIds,['alpha']);
  assert.equal(control.actions.some(a=>a.url==='/api/launch'||a.url==='/api/reveal'),false);
});

test('picker keeps its draft and visible error after failure; retry saves it',async t=>{
  const {w,$,control,checked}=await fixture(t,{preferences:{categories:[...builtins(),custom('work','工作')]}});
  $('[aria-label="Gamma 的更多操作"]').click();$('#context-menu [data-action="categories"]').click();checked('work',true);
  control.plans.push(()=>({status:500}));$('#save-picker').click();await tick();
  assert.equal($('#picker-dialog').open,true);assert.equal($('#picker-error').hidden,false);assert.equal($('#picker-list [data-option="work"]').checked,true);assert.deepEqual(control.preferences.categories.find(c=>c.id==='work').appIds,[]);
  $('#save-picker').click();await tick();assert.equal($('#picker-dialog').open,false);assert.deepEqual(control.preferences.categories.find(c=>c.id==='work').appIds,['gamma']);
});

test('failed queued edits roll back to the last confirmation, while a later success is rebased',async t=>{
  const {w,$,control,input}=await fixture(t);
  const first=deferred();control.plans.push(async()=>{await first.promise;return {status:500}},()=>({status:500}));
  input('#icon-size','104');input('#icon-size','112');assert.equal($('#icon-size').value,'112');first.resolve();await tick(35);
  assert.equal($('#icon-size').value,'80');assert.equal(control.preferences.iconSize,80);
  const second=deferred();control.plans.push(async()=>{await second.promise;return {status:500}});
  input('#icon-size','104');input('#icon-size','96');second.resolve();await tick(35);
  assert.equal($('#icon-size').value,'96');assert.equal(control.preferences.iconSize,96);
  assert.equal(control.errors.length,3);
});

test('closing waits for edits added while an earlier save is pending',async t=>{
  const {$,control,input}=await fixture(t);
  const first=deferred(), second=deferred();
  control.plans.push(async()=>{await first.promise;return {}},async()=>{await second.promise;return {}});
  input('#icon-size','96');$('#organize-button').click();input('#icon-size','112');
  first.resolve();await tick(30);
  assert.equal(control.saves.length,2);assert.equal(control.preferences.iconSize,96);assert.equal(control.closed,0);
  second.resolve();await tick(30);
  assert.equal(control.preferences.iconSize,112);assert.equal(control.closed,1);
});

test('closing drains later edits and preserves a failure from any pending save',async t=>{
  for(const failing of ['first','second']) await t.test(`${failing} save fails`,async t=>{
    const {$,control,input}=await fixture(t);
    const first=deferred(), second=deferred();
    control.plans.push(async()=>{await first.promise;return failing==='first'?{status:500}:{}},async()=>{await second.promise;return failing==='second'?{status:500}:{}});
    input('#icon-size','96');$('#organize-button').click();input('#icon-size','112');
    first.resolve();await tick(30);assert.equal(control.closed,0);assert.equal(control.saves.length,2);
    second.resolve();await tick(30);
    assert.equal(control.closed,0);assert.equal(control.preferences.iconSize,failing==='first'?112:96);assert.equal(control.errors.length,1);
    $('#organize-button').click();await tick(30);assert.equal(control.closed,1);
  });
});

test('edits during the closing animation finish saving before the window closes',async t=>{
  for(const fails of [false,true]) await t.test(fails?'save fails':'save succeeds',async t=>{
    const {w,$,control,input}=await fixture(t);
    w.matchMedia=()=>({matches:false});
    $('#organize-button').click();await tick();assert.equal(w.document.body.classList.contains('leaving'),true);
    const gate=deferred();control.plans.push(async()=>{await gate.promise;return fails?{status:500}:{}});
    input('#icon-size','112');await tick(200);assert.equal(control.closed,0);
    gate.resolve();await tick(30);
    assert.equal(control.closed,fails?0:1);assert.equal(control.preferences.iconSize,fails?80:112);
    if(fails) {
      assert.equal(w.document.body.classList.contains('leaving'),false);
      $('#organize-button').click();await tick(200);assert.equal(control.closed,1);
    }
  });
});

test('a save failure completed during the closing animation keeps the manager interactive',async t=>{
  const {w,$,control,input}=await fixture(t);
  w.matchMedia=()=>({matches:false});
  $('#organize-button').click();await tick();
  control.plans.push(()=>({status:500}));input('#icon-size','112');await tick(30);assert.equal(control.errors.length,1);
  await tick(200);
  assert.equal(control.closed,0);assert.equal(control.preferences.iconSize,80);assert.equal(w.document.body.inert,false);assert.equal(w.document.body.classList.contains('leaving'),false);
});

test('native close preparation shares its save barrier and restores input after failure',async t=>{
  const {w,control,input}=await fixture(t);
  const gate=deferred();control.plans.push(async()=>{await gate.promise;return {status:500}});input('#icon-size','96');
  const first=w.__gopadPrepareClose(), duplicate=w.__gopadPrepareClose();
  assert.equal(first,duplicate);assert.equal(w.document.body.inert,true);assert.equal(control.closed,0);
  gate.resolve();assert.equal(await first,false);assert.equal(w.document.body.inert,false);assert.equal(w.document.body.classList.contains('leaving'),false);
  assert.equal(await w.__gopadPrepareClose(),true);assert.equal(w.document.body.inert,true);assert.equal(control.closed,0);
  w.__gopadCancelClose();assert.equal(w.document.body.inert,false);assert.equal(w.document.body.classList.contains('leaving'),false);
});

test('cancelled native preparation cannot approve a later close attempt',async t=>{
  const {w,control,input}=await fixture(t);
  const gate=deferred();control.plans.push(async()=>{await gate.promise;return {}});input('#icon-size','96');
  const old=w.__gopadPrepareClose();w.__gopadCancelClose();assert.equal(w.document.body.inert,false);
  const retry=w.__gopadPrepareClose();assert.notEqual(old,retry);assert.equal(w.document.body.inert,true);
  gate.resolve();assert.equal(await old,false);assert.equal(await retry,true);assert.equal(w.document.body.inert,true);
  w.__gopadCancelClose();assert.equal(w.document.body.inert,false);
});

test('a failed quit request restores input after draining pending edits',async t=>{
  const {w,$,control,input}=await fixture(t);
  $('#sidebar-settings').click();await tick();
  const gate=deferred();control.plans.push(async()=>{await gate.promise;return {}});
  control.actionFailures['/api/quit']={error:'quit failed'};
  input('#icon-size','96');$('#quit-button').click();assert.equal(w.document.body.inert,true);
  input('#icon-size','112');assert.equal(control.actions.filter(action=>action.url==='/api/quit').length,0);
  gate.resolve();await tick(30);
  assert.equal(control.preferences.iconSize,112);assert.equal(control.actions.filter(action=>action.url==='/api/quit').length,1);
  assert.equal(w.document.body.inert,false);assert.equal(w.document.body.classList.contains('leaving'),false);
});

test('canonical responses rebase pending category edits without discarding them',async t=>{
  const {w,$,control,input,submitName}=await fixture(t);
  const gate=deferred();control.plans.push(async candidate=>{await gate.promise;return {preferences:{...candidate,categories:[...candidate.categories,custom('server-section','服务端归一化')]}}});
  input('#icon-size','104');$('#edit-categories').click();$('#new-category').click();submitName('待保存分区');
  assert.ok($('[data-view]').parentElement);gate.resolve();await tick(40);
  assert.ok(control.preferences.categories.some(c=>c.id==='server-section'));assert.ok(control.preferences.categories.some(c=>c.name==='待保存分区'));
  assert.ok([...$('#navigation').children].some(tab=>tab.textContent.includes('待保存分区')));
});

test('late picker save cannot close or alter a newly opened application picker',async t=>{
  const {w,$,control,checked,key}=await fixture(t,{preferences:{categories:[...builtins(),custom('work','工作')]}});
  $('[aria-label="Gamma 的更多操作"]').click();$('#context-menu [data-action="categories"]').click();checked('work',true);
  const gate=deferred();control.plans.push(async()=>{await gate.promise;return {status:500}});$('#save-picker').click();
  key(w.document,'Escape');await tick();$('[aria-label="Alpha 的更多操作"]').click();$('#context-menu [data-action="categories"]').click();
  assert.equal($('#picker-context').textContent,'Alpha');gate.resolve();await tick(30);
  assert.equal($('#picker-dialog').open,true);assert.equal($('#picker-context').textContent,'Alpha');assert.equal($('#picker-error').hidden,true);assert.equal($('#save-picker').disabled,false);assert.equal($('#picker-list [data-option="work"]').checked,false);
});

test('64 custom sections, long Unicode names and hidden builtins remain normalized without deleting legitimate sections',async t=>{
  const categories=[...builtins(),...Array.from({length:64},(_,index)=>custom(`section-${index}`,'🌙'.repeat(40)))];
  const {w,$,control,key}=await fixture(t,{preferences:{categories}});
  assert.equal($('#navigation').children.length,67);$('#edit-categories').click();assert.equal($('#new-category').disabled,true);
  assert.equal($('#category-list').children.length,68);
  assert.equal($('#category-list [data-category-row="section-63"] .category-name-button').textContent,'🌙'.repeat(40));
  key(w.document,'Escape');await tick();$('[data-view="section-63"]').click();assert.equal($('#navigation [aria-selected="true"]').dataset.view,'section-63');
});

test('web catalogs cover every static message, have matching placeholders, and UI source has no embedded copy',()=>{
  const catalogs={};
  for(const locale of ['en','zh-CN']) catalogs[locale]=JSON.parse(fs.readFileSync(path.join(root,`internal/i18n/locales/${locale}/web.json`),'utf8'));
  assert.deepEqual(Object.keys(catalogs.en).sort(),Object.keys(catalogs['zh-CN']).sort());
  const placeholders=value=>[...value.matchAll(/\{([A-Za-z][A-Za-z0-9_]*)\}/g)].map(match=>match[1]).sort();
  for(const [key,value] of Object.entries(catalogs.en)) {
    assert.ok(key.startsWith('web.'));assert.equal(typeof value,'string');assert.ok(value.trim());assert.ok(catalogs['zh-CN'][key].trim());
    assert.deepEqual(placeholders(value),placeholders(catalogs['zh-CN'][key]),`placeholder mismatch ${key}`);
  }
  const html=fs.readFileSync(path.join(root,'frontend/index.html'),'utf8');
  const document=new JSDOM(html).window.document;
  for(const element of document.querySelectorAll('*')) for(const attr of element.attributes) {
    if(attr.name.startsWith('data-i18n') && attr.name!=='data-i18n-meta') assert.ok(catalogs.en[attr.value],`missing static key ${attr.value}`);
    if(['aria-label','title','placeholder'].includes(attr.name) && attr.value) assert.ok(element.hasAttribute(`data-i18n-${attr.name}`),`hardcoded ${attr.name}`);
  }
  const walker=document.createTreeWalker(document.body,4);
  while(walker.nextNode()) {
    const text=walker.currentNode.textContent.trim();
    if(text) assert.match(text,/^(?:Tab|—|\d+(?:\s?px)?)$/,`untranslated static copy: ${text}`);
  }
  for(const file of ['app.js','components/ui.js','components/i18n.js']) {
    const source=fs.readFileSync(path.join(root,'frontend',file),'utf8');
    assert.doesNotMatch(source,/[\p{Script=Han}]/u,`embedded Chinese copy in ${file}`);
    assert.doesNotMatch(source,/\.(?:textContent|title)\s*=\s*['"][A-Za-z][^'"\n]*['"]/,`embedded English label in ${file}`);
    for(const [,key] of source.matchAll(/['"](web\.[A-Za-z0-9_.]+)['"]/g)) assert.ok(catalogs.en[key],`missing live key ${key}`);
  }
});

test('English translates static labels, menu, aria and builtins while preserving custom names and Dev metadata',async t=>{
  const {w,$,control,event}=await fixture(t,{preferences:{language:'en',categories:[...builtins(),custom('work','工作台',['gamma'])],lastCategory:'work'}});
  assert.equal(w.document.documentElement.lang,'en');assert.equal(w.document.title,'gopad Dev · Manage Apps');assert.equal($('h1').textContent,'Apps');
  assert.equal($('#search').placeholder,'Search apps');assert.equal($('#organize-button').textContent,'Done');assert.equal($('#launchpad-shortcut').textContent,'⌃ ⌥ ⇧ Space');
  assert.ok($('[data-view="all"]').textContent.startsWith('All'));assert.ok($('[data-view="work"]').textContent.startsWith('工作台'));
  $('[aria-label="More actions for Gamma"]').click();assert.ok($('#context-menu').textContent.includes('Show in Finder'));assert.ok($('#context-menu').textContent.includes('Add to sections…'));
  $('#context-menu [data-action="copy"]').click();await tick();assert.ok($('#toast').textContent.includes('App path copied'));
  assert.equal(control.preferences.lastCategory,'work');
});

test('language save and live locale refresh preserve current tab, search, scroll and nested drafts',async t=>{
  const {w,$,control,event,input,checked,active}=await fixture(t,{preferences:{categories:[...builtins(),custom('work','工作台',['gamma'])],lastCategory:'work'}});
  input('#search','Gamma');$('#apps-scroll').scrollTop=123;
  $('[aria-label="Gamma 的更多操作"]').click();$('#context-menu [data-action="categories"]').click();checked('work',false);
  $('#picker-new-category').click();$('#category-name').value='未完成的名字';
  control.preferences.language='en';w.dispatchEvent(new w.Event('mygo:locale'));await tick();
  assert.equal(w.document.documentElement.lang,'en');assert.equal(active(),'work');assert.equal($('#search').value,'Gamma');assert.equal($('#apps-scroll').scrollTop,123);
  assert.equal($('#category-name-dialog').open,true);assert.equal($('#category-name').value,'未完成的名字');assert.equal($('#category-name-title').textContent,'New section');
  assert.equal($('#picker-list [data-option="work"]').checked,false);assert.equal($('#picker-search').value,'');assert.equal(control.saves.length,0);
});

test('language choice persists, failed choice rolls back, and managing never overwrites remembered launchpad category',async t=>{
  const {w,$,control,event,input,active}=await fixture(t,{preferences:{categories:[...builtins(),custom('work','工作台',['gamma'])],lastCategory:'work'}});
  $('[data-view="all"]').click();$('#sidebar-settings').click();await tick();
  $('#language').value='en';$('#language').dispatchEvent(event('change'));await tick();
  assert.equal(control.preferences.language,'en');assert.equal(w.document.documentElement.lang,'en');assert.equal(active(),'all');assert.equal(control.preferences.lastCategory,'work');
  control.plans.push(()=>({status:500}));$('#language').value='zh-CN';$('#language').dispatchEvent(event('change'));await tick();
  assert.equal($('#language').value,'en');assert.equal(w.document.documentElement.lang,'en');assert.equal(control.preferences.language,'en');
  assert.ok($('#toast').textContent.startsWith('Not saved:'));
  assert.ok(control.saves.every(snapshot=>snapshot.lastCategory==='work'));
});

test('unknown language is preserved while UI falls back, and system mode follows locale refresh',async t=>{
  const {w,$,control,event,input}=await fixture(t,{preferences:{language:'fr-CA'}});
  assert.equal(w.document.documentElement.lang,'en');assert.equal($('#language').value,'fr-CA');assert.ok($('#language [data-language-unknown]').textContent.includes('fr-CA'));
  input('#icon-size','96');await tick();assert.equal(control.preferences.language,'fr-CA');
  $('#language').value='system';$('#language').dispatchEvent(event('change'));await tick();assert.equal(w.document.documentElement.lang,'zh-CN');
  control.systemLocale='en';w.dispatchEvent(new w.Event('mygo:locale'));await tick();assert.equal(w.document.documentElement.lang,'en');assert.equal(control.preferences.language,'system');
});

test('language refresh failure keeps the saved preference and intact form with accurate feedback',async t=>{
  const {w,$,control,event}=await fixture(t);
  $('#sidebar-settings').click();await tick();control.i18nPlans.push(()=>({status:500}));
  $('#language').value='en';$('#language').dispatchEvent(event('change'));await tick();
  assert.equal(control.preferences.language,'en');assert.equal($('#language').value,'en');assert.equal(w.document.documentElement.lang,'zh-CN');
  assert.ok($('#toast').textContent.includes('语言已保存'));assert.equal($('#settings-dialog').open,true);
});

test('updater uses the native window, waits for pending edits, and keeps automatic checks outside app preferences',async t=>{
  const {w,$,control,input}=await fixture(t,{preferences:{language:'en'}});
  $('#sidebar-settings').click();await tick();
  assert.equal($('#current-version').textContent,'0.1.0');assert.equal($('#check-updates').disabled,false);assert.equal($('#update-status').hidden,true);
  $('#automatic-updates').click();await tick();assert.equal(control.updateState.automaticChecks,false);assert.equal($('#automatic-updates').getAttribute('aria-checked'),'false');assert.equal(control.saves.length,0);
  const gate=deferred();control.plans.push(async()=>{await gate.promise;return {}});
  input('#icon-size','96');$('#check-updates').click();await tick();assert.equal(control.actions.filter(action=>action.url==='/api/update/check').length,0);
  gate.resolve();await tick(25);assert.equal(control.actions.filter(action=>action.url==='/api/update/check').length,1);assert.equal($('#settings-dialog').open,true);
  assert.ok(control.saves.every(snapshot=>!Object.hasOwn(snapshot,'autoUpdate')));
  control.updatePlans.push(()=>({status:500}));$('#automatic-updates').click();await tick();assert.equal($('#automatic-updates').getAttribute('aria-checked'),'false');assert.equal(control.updateState.automaticChecks,false);
  assert.ok($('#toast').textContent.includes('The update preference could not be saved'));
});

test('unconfigured updater never claims latest, disables installation controls, and reports its actual state',async t=>{
  const {w,$,control}=await fixture(t,{preferences:{language:'en'},updateState:{status:'not_configured',currentVersion:'dev'}});
  $('#sidebar-settings').click();await tick();
  assert.equal($('#current-version').textContent,'dev');assert.equal($('#check-updates').disabled,true);assert.equal($('#automatic-updates').disabled,true);
  assert.equal($('#update-status').hidden,false);assert.ok($('#update-status').textContent.includes('not configured'));assert.equal(control.actions.some(action=>action.url==='/api/update/install'),false);
});

test('localized API errors use their stable key and reformat when language changes',async t=>{
  const {w,$,control}=await fixture(t,{preferences:{language:'en'}});
  control.actionFailures['/api/launch']={error:'不可展示的旧语言说明',errorKey:'error.appNotFound',detail:'private diagnostic path'};
  $('[aria-label="More actions for Gamma"]').click();$('#context-menu [data-action="launch"]').click();await tick();
  assert.ok($('#toast').textContent.includes('The application could not be found'));assert.ok(!$('#toast').textContent.includes('private diagnostic'));assert.ok(!$('#toast').textContent.includes('不可展示'));
  control.preferences.language='zh-CN';w.dispatchEvent(new w.Event('mygo:locale'));await tick();
  assert.ok($('#toast').textContent.includes('找不到该应用'));assert.ok(!$('#toast').textContent.includes('Action failed'));
});

test('failed pending changes prevent opening the updater until an explicit retry',async t=>{
  const {w,$,control,input}=await fixture(t);
  $('#sidebar-settings').click();await tick();
  const gate=deferred();control.plans.push(async()=>{await gate.promise;return {status:500}});
  input('#icon-size','96');$('#check-updates').click();gate.resolve();await tick(25);
  assert.equal(control.actions.filter(action=>action.url==='/api/update/check').length,0);assert.equal($('#settings-icon-size').value,'80');assert.equal($('#check-updates').disabled,false);
  $('#check-updates').click();await tick();assert.equal(control.actions.filter(action=>action.url==='/api/update/check').length,1);
});

test('one visible section shows apps directly, keeps section management and uses a visible-independent accessible label',async t=>{
  const categories=builtins().map(category=>({...category,visible:category.id==='all'}));
  const {w,$,control,event,key,ids,submitName}=await fixture(t,{preferences:{language:'en',categories}});
  assert.deepEqual(ids(),['gamma','alpha','beta']);assert.equal($('#navigation').hidden,true);assert.equal($('#category-shortcut').hidden,true);
  assert.equal($('#edit-categories').hidden,false);assert.equal($('#apps-scroll').getAttribute('aria-labelledby'),null);assert.equal($('#apps-scroll').getAttribute('aria-label'),'All');
  assert.equal(key($('#search'),'Tab').defaultPrevented,false);
  $('#edit-categories').click();assert.equal($('#categories-dialog').open,true);$('#new-category').click();submitName('Work');await tick();
  const category=control.preferences.categories.find(category=>category.name==='Work');assert.ok(category);
  assert.equal($('#navigation').hidden,false);assert.equal($('#category-shortcut').hidden,false);assert.equal($('#apps-scroll').getAttribute('aria-labelledby'),'category-tab-all');assert.equal($('#apps-scroll').hasAttribute('aria-label'),false);
  $(`[data-category-row="${category.id}"] [data-category-action="visibility"]`).click();await tick();
  assert.equal($('#navigation').hidden,true);assert.equal($('#category-shortcut').hidden,true);assert.equal($('#apps-scroll').getAttribute('aria-label'),'All');
});

test('a single custom section and temporary hidden management never label a panel with a hidden tab',async t=>{
  const categories=[...builtins().map(category=>({...category,visible:false})),custom('work','工作台',['gamma'])];
  const {w,$,control,key,ids}=await fixture(t,{preferences:{categories,lastCategory:'work'}});
  assert.deepEqual(ids(),['gamma']);assert.equal($('#navigation').hidden,true);assert.equal($('#apps-scroll').getAttribute('aria-label'),'工作台');assert.equal($('#apps-scroll').hasAttribute('aria-labelledby'),false);
  $('#sidebar-settings').click();$('#manage-hidden').click();await tick();
  assert.deepEqual(ids(),['delta']);assert.equal($('#navigation').hidden,true);assert.equal($('#category-shortcut').hidden,true);assert.equal($('#apps-scroll').getAttribute('aria-label'),'隐藏');assert.equal($('#apps-scroll').hasAttribute('aria-labelledby'),false);
  $('#edit-categories').click();assert.equal($('#categories-dialog').open,true);assert.ok($('[data-category-row="work"]'));
});

test('toolbar inset follows actual icon frames at normal and minimum widths for every supported icon size',async t=>{
  const {w,$,input}=await fixture(t);
  const inset=w.GopadUI.gridInlineInset;
  const cases=[
    {width:1020,min:104,size:48,gap:8,expected:29.111111},
    {width:1020,min:132,size:80,gap:8,expected:29.428571},
    {width:1020,min:164,size:112,gap:8,expected:42.8},
    {width:714,min:104,size:48,gap:4,expected:33.833333},
    {width:714,min:132,size:80,gap:4,expected:29.8},
    {width:714,min:164,size:112,gap:4,expected:31.75},
    {width:709,min:164,size:112,gap:4,expected:31.125},
  ];
  for(const item of cases) assert.ok(Math.abs(inset(item.width,item.min,item.size,item.gap)-item.expected)<.00001,JSON.stringify(item));
  Object.defineProperty($('#app-grid'),'clientWidth',{value:714,configurable:true});$('#app-grid').style.columnGap='4px';
  w.dispatchEvent(new w.Event('resize'));assert.equal(w.document.documentElement.style.getPropertyValue('--grid-inline-inset'),'29.800px');
  input('#icon-size','48');await tick();assert.equal(w.document.documentElement.style.getPropertyValue('--grid-inline-inset'),'33.833px');
  input('#icon-size','112');await tick();assert.equal(w.document.documentElement.style.getPropertyValue('--grid-inline-inset'),'31.750px');
  Object.defineProperty($('#apps-scroll'),'offsetWidth',{value:714,configurable:true});Object.defineProperty($('#app-grid'),'clientWidth',{value:709,configurable:true});
  w.dispatchEvent(new w.Event('resize'));assert.equal(w.document.documentElement.style.getPropertyValue('--grid-inline-inset'),'31.125px');assert.equal(w.document.documentElement.style.getPropertyValue('--grid-inline-end-inset'),'36.125px');
  assert.equal(inset(0,164,112,4),0);
});
