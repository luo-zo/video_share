// 视图模块只产出纯描述对象，渲染交给 view-kit；这里用最小 DOM 替身让测试能够
// 真正执行生产渲染逻辑，从而验证不受信任的文本只会进入 textContent。
function createElement(tag) {
  return {
    tag,
    className: '',
    id: '',
    textContent: '',
    attrs: {},
    props: {},
    dataset: {},
    style: {},
    children: [],
    listeners: {},
    setAttribute(name, value) { this.attrs[name] = String(value); },
    removeAttribute(name) { delete this.attrs[name]; },
    append(...nodes) { this.children.push(...nodes); },
    replaceChildren(...nodes) { this.children = [...nodes]; },
    addEventListener(type, handler) { (this.listeners[type] ||= []).push(handler); },
  };
}

export function fakeDocument() {
  return {
    createElement,
    createTextNode: (value) => ({ tag: '#text', textContent: String(value) }),
    createDocumentFragment: () => createElement('#fragment'),
  };
}

export function collectText(node) {
  if (!node) return '';
  const own = node.textContent || '';
  return own + (node.children || []).map(collectText).join('');
}

export function collectAttrs(node, out = []) {
  if (!node) return out;
  for (const value of Object.values(node.attrs || {})) out.push(value);
  for (const child of node.children || []) collectAttrs(child, out);
  return out;
}
