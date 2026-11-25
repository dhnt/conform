// Browser entry point: walkInventory(document). Uses DOM tree primitives only.
function walkInventory(root) {
  function tag(n) { return (n.tagName || '').toLowerCase(); }
  function attr(n, k) { return n.getAttribute ? n.getAttribute(k) || '' : ''; }
  function has(n, k) { return n.getAttribute && n.getAttribute(k) !== null; }
  function cls(n, k) { return attr(n, 'class').split(/\s+/).includes(k); }
  function hidden(n) {
    if (!n.tagName) return false;
    if (has(n, 'hidden') || attr(n, 'aria-hidden').toLowerCase() === 'true' ||
        tag(n) === 'input' && attr(n, 'type').toLowerCase() === 'hidden') return true;
    return attr(n, 'style').toLowerCase().split(';').some(function (d) {
      var p = d.split(':');
      return p.length >= 2 && p[0].trim() === 'display' && p[1].trim().startsWith('none');
    });
  }
  function visit(n, fn) {
    fn(n);
    if (hidden(n)) return;
    Array.from(n.childNodes || []).forEach(function (c) { visit(c, fn); });
  }
  function walk(n, ancestorHidden, fn) {
    if (tag(n) === 'form' && !ancestorHidden) fn(n);
    Array.from(n.childNodes || []).forEach(function (c) { walk(c, ancestorHidden || hidden(n), fn); });
  }
  function clean(s) { return s.trim().replace(/\s+/g, ' '); }
  function visibleText(n, option) {
    if (hidden(n) || option && cls(n, 'desk-recommended')) return '';
    if (!n.tagName && n.textContent) return n.textContent;
    return Array.from(n.childNodes || []).map(function (c) { return visibleText(c, option); }).join('');
  }
  function allText(n) {
    if (!n.tagName && n.textContent) return n.textContent;
    return Array.from(n.childNodes || []).map(allText).join('');
  }
  function visitFields(n, inField, fn) {
    if (hidden(n)) return;
    var isField = tag(n) === 'div' && has(n, 'data-field');
    if (isField && !inField) fn(n);
    Array.from(n.childNodes || []).forEach(function (c) { visitFields(c, inField || isField, fn); });
  }
  function field(n, form) {
    var f = {id: attr(n, 'data-field'), label: '', kind: '', required: attr(n, 'aria-required').toLowerCase() === 'true', options: []};
    var dk = attr(n, 'data-kind'), kinds = [], label = null;
    visit(n, function (c) {
      if (c === n || hidden(c)) return;
      var t = tag(c);
      if (t === 'label') {
        if (!label && !cls(c, 'desk-option')) label = c;
        visit(c, function (child) {
          if (tag(child) === 'input' && !attr(child, 'value') &&
              ['radio', 'checkbox'].includes(attr(child, 'type').toLowerCase())) {
            var fallback = clean(visibleText(c, true));
            if (fallback) f.options.push(fallback);
          }
        });
      }
      if (t === 'textarea' || t === 'select' || t === 'input') {
        var k = t === 'input' ? (attr(c, 'type').toLowerCase() || 'text') : t;
        kinds.push(k);
        if (has(c, 'required') || attr(c, 'aria-required').toLowerCase() === 'true') f.required = true;
        if (k === 'radio' || k === 'checkbox') {
          var v = attr(c, 'value');
          if (v) f.options.push(v);
        }
      }
      if (t === 'option' && attr(c, 'value').trim()) f.options.push(attr(c, 'value').trim());
    });
    if (!label && f.id) visit(form, function (c) {
      if (!label && tag(c) === 'label' && attr(c, 'for') === f.id) label = c;
    });
    if (label) {
      var raw = visibleText(label, false);
      f.label = clean(raw).replace(/\s*\*+\s*$/, '').trim();
      if (allText(label).includes('*')) f.required = true;
    }
    if (!kinds.length) return null;
    if (kinds.includes('textarea')) f.kind = 'textarea';
    else if (kinds.includes('file')) f.kind = 'file';
    else if (kinds.includes('checkbox')) f.kind = 'multi';
    else if (kinds.includes('radio') || kinds.includes('select')) {
      f.kind = dk === 'yes-no' || dk !== 'choice' && f.options.length === 2 &&
        f.options[0] === 'yes' && f.options[1] === 'no' ? 'yes-no' : 'choice';
    } else f.kind = dk || 'text';
    return f;
  }
  var out = {fields: [], has_save_button: false, has_submit_button: false, buttons: []};
  walk(root, false, function (form) {
    if (hidden(form)) return;
    visit(form, function (n) {
      if (hidden(n)) return;
      if (tag(n) === 'button') {
        var label = clean(visibleText(n, false));
        out.buttons.push(label);
        if (has(n, 'data-desk-save') || label.toLowerCase().includes('save')) out.has_save_button = true;
        if (has(n, 'data-desk-submit') || attr(n, 'type').toLowerCase() === 'submit' ||
            label.toLowerCase().includes('submit')) out.has_submit_button = true;
      }
    });
    visitFields(form, false, function (n) {
      var f = field(n, form);
      if (f) out.fields.push(f);
    });
  });
  return out;
}
