// Browser entry point: walkInventory(document). Uses DOM tree primitives only.
function walkInventory(root) {
  function key(n) { return n && (n.walkID || n); }
  function tag(n) { return (n.tagName || '').toLowerCase(); }
  function attr(n,k) { return n && n.getAttribute ? n.getAttribute(k) || '' : ''; }
  function has(n,k) { return !!(n && n.getAttribute && n.getAttribute(k) !== null); }
  function cls(n,k) { return attr(n,'class').split(/\s+/).includes(k); }
  function hidden(n) {
    if (!n || !n.tagName) return false;
    if (has(n,'hidden') || attr(n,'aria-hidden').toLowerCase()==='true' || tag(n)==='input' && attr(n,'type').toLowerCase()==='hidden') return true;
    return attr(n,'style').toLowerCase().split(';').some(function(d) { var p=d.split(':'); return p.length>=2 && (p[0].trim()==='display' && p[1].trim().startsWith('none') || p[0].trim()==='visibility' && p[1].trim().startsWith('hidden')); });
  }
  function visit(n,fn) { if (hidden(n)) return; fn(n); Array.from(n.childNodes||[]).forEach(function(c){visit(c,fn);}); }
  function clean(s) { return s.trim().replace(/\s+/g,' '); }
  function visibleText(n,option) { if (hidden(n) || option && cls(n,'desk-recommended')) return ''; if (!n.tagName && n.textContent) return n.textContent; return Array.from(n.childNodes||[]).map(function(c){return visibleText(c,option);}).join(''); }
  function allText(n) { if (!n.tagName && n.textContent) return n.textContent; return Array.from(n.childNodes||[]).map(allText).join(''); }
  function trimLabel(s) { return clean(s).replace(/\s*\*+\s*$/,'').trim(); }
  function labelText(n) { return trimLabel(visibleText(n,false)); }
  function required(n) { return has(n,'required') || attr(n,'aria-required').toLowerCase()==='true'; }
  function inputType(n) { if (tag(n)!=='input') return tag(n); return attr(n,'type').toLowerCase() || 'text'; }
  function control(n) { return tag(n)==='textarea' || tag(n)==='select' || tag(n)==='input' && !['hidden','submit','button','reset','image'].includes(inputType(n)); }
  function ancestor(n,form,pred) { for (var p=n.parentNode;p && key(p)!==key(form);p=p.parentNode) if (pred(p)) return p; return null; }
  function fieldWrap(n,form) { return ancestor(n,form,function(p){return has(p,'data-field');}); }
  function groupWrap(n,form) { return fieldWrap(n,form) || ancestor(n,form,function(p){return tag(p)==='fieldset' || ['radiogroup','group'].includes(attr(p,'role'));}); }
  function first(n,pred) { var found=null; visit(n,function(c){if (!found && pred(c)) found=c;}); return found; }
  function byID(form,id) { return id ? first(form,function(n){return attr(n,'id')===id;}) : null; }
  function labelledBy(n,form) { return trimLabel(attr(n,'aria-labelledby').split(/\s+/).map(function(id){var ref=byID(form,id);return ref ? labelText(ref) : '';}).join(' ')); }
  function findLabel(form,id) { return id ? first(form,function(n){return tag(n)==='label' && attr(n,'for')===id;}) : null; }
  function legend(n) { return n ? first(n,function(c){return tag(c)==='legend';}) : null; }
  function fieldLabel(n,group,form,grouped) {
    if (grouped) return group ? findLabel(form,attr(group,'id')) : null;
    return findLabel(form,attr(n,'id')) || ancestor(n,form,function(p){return tag(p)==='label';});
  }
  function groupLabel(group) {
    if (!group) return null;
    return legend(group) || (has(group,'data-field') ? first(group,function(c){return tag(c)==='label' && !cls(c,'desk-option') && !first(c,control);}) : null);
  }
  function optionLabel(n,form) {
    var l=findLabel(form,attr(n,'id')); if (l && labelText(l)) return labelText(l);
    l=ancestor(n,form,function(p){return tag(p)==='label';}); if (l && clean(visibleText(l,true))) return clean(visibleText(l,true));
    return labelledBy(n,form) || clean(attr(n,'aria-label')) || clean(attr(n,'value'));
  }
  function fieldID(n,group) { return group && has(group,'data-field') ? attr(group,'data-field') : has(n,'data-field') ? attr(n,'data-field') : attr(n,'name') || attr(n,'id'); }
  var out={fields:[],buttons:[]};
  visit(root,function(form) {
    if (tag(form)!=='form') return;
    var seen=new Map(), named=new Set();
    visit(form,function(n) {
      if (tag(n)==='button' || tag(n)==='input' && ['submit','button','reset'].includes(inputType(n))) {
        var button=tag(n)==='input' ? clean(attr(n,'value')) : clean(visibleText(n,false));
        if (!button && tag(n)==='input') button=inputType(n)==='submit' ? 'Submit' : inputType(n)==='reset' ? 'Reset' : '';
        button=button || clean(attr(n,'aria-label')); if (button) out.buttons.push(button);
      }
      if (!control(n)) return;
      var kind=inputType(n), grouped=kind==='radio' || kind==='checkbox';
      var group=grouped ? groupWrap(n,form) : fieldWrap(n,form) || ancestor(n,form,function(p){return tag(p)==='fieldset';});
      if (grouped) { if (group) { var kinds=seen.get(key(group)) || new Set(); if (kinds.has(kind)) return; kinds.add(kind); seen.set(key(group),kinds); } else if (attr(n,'name')) { var nameKey=kind+':'+attr(n,'name'); if (named.has(nameKey)) return; named.add(nameKey); } }
      var members=[n];
      if (grouped) { members=[]; visit(form,function(c){ if (!control(c) || inputType(c)!==kind) return; if (group ? key(groupWrap(c,form))===key(group) : key(c)===key(n) || !groupWrap(c,form) && attr(c,'name')===attr(n,'name') && !!attr(n,'name')) members.push(c); }); }
      var f={id:fieldID(n,group),label:'',kind:'',required:!!(group && required(group)),options:[]};
      members.forEach(function(c){f.required=f.required || required(c);});
      var label=fieldLabel(n,group,form,grouped);
      if (label) { f.label=labelText(label); if (allText(label).includes('*')) f.required=true; }
      if (!f.label) [group,n].forEach(function(source){if (source && !f.label) f.label=labelledBy(source,form) || trimLabel(attr(source,'aria-label'));});
      if (!f.label) { var fallback=groupLabel(group); if (fallback) { f.label=labelText(fallback); if (allText(fallback).includes('*')) f.required=true; } }
      if (!f.label && grouped && members.length===1) f.label=optionLabel(n,form);
      if (kind==='select') visit(n,function(c){if (tag(c)==='option' && !(has(c,'value') && !attr(c,'value').trim())) {var s=clean(visibleText(c,false)) || clean(attr(c,'value')); if (s) f.options.push(s);}});
      if (grouped) members.forEach(function(c){var s=optionLabel(c,form);if (s) f.options.push(s);});
      if (kind==='textarea') f.kind='textarea';
      else if (kind==='select') { f.kind=has(n,'multiple') ? 'multi' : 'choice'; if (f.kind==='choice' && f.options.length===2 && ((f.options[0].toLowerCase()==='yes' && f.options[1].toLowerCase()==='no') || (f.options[0].toLowerCase()==='no' && f.options[1].toLowerCase()==='yes'))) f.kind='yes-no'; }
      else if (kind==='radio') { f.kind='choice'; if (f.options.length===2 && ((f.options[0].toLowerCase()==='yes' && f.options[1].toLowerCase()==='no') || (f.options[0].toLowerCase()==='no' && f.options[1].toLowerCase()==='yes'))) f.kind='yes-no'; }
      else if (kind==='checkbox') { f.kind=members.length===1 ? 'yes-no' : 'multi'; if (members.length===1) f.options=[]; }
      else if (kind==='file') f.kind='file'; else f.kind='text';
      f.kind=group && attr(group,'data-kind') || attr(n,'data-kind') || f.kind;
      out.fields.push(f);
    });
  });
  return out;
}
