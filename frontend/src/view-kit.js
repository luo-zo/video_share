// 视图模块只产出纯描述对象，DOM 落地集中在这里：文本一律走 textContent，属性只经
// setAttribute 写入，任何来自服务端或用户的内容都不会进入 attrs，也绝不使用 innerHTML。
export function element(tag, {
  className = '', id = '', text = '', attrs = {}, props = {}, dataset = {}, children = [], on = {},
} = {}) {
  return Object.freeze({
    tag,
    className,
    id,
    text: String(text ?? ''),
    attrs: Object.freeze({ ...attrs }),
    props: Object.freeze({ ...props }),
    dataset: Object.freeze({ ...dataset }),
    children: Object.freeze(children.filter((child) => child !== null && child !== undefined && child !== false)),
    on: Object.freeze({ ...on }),
  });
}

function applyAttrs(node, attrs) {
  for (const [name, value] of Object.entries(attrs)) {
    if (value === null || value === undefined || value === false) continue;
    node.setAttribute(name, value === true ? '' : String(value));
  }
}

export function renderNode(descriptor, document = globalThis.document) {
  if (descriptor === null || descriptor === undefined || descriptor === false) return null;
  if (typeof descriptor === 'string' || typeof descriptor === 'number') {
    return document.createTextNode(String(descriptor));
  }
  const node = document.createElement(descriptor.tag);
  if (descriptor.className) node.className = descriptor.className;
  if (descriptor.id) node.id = descriptor.id;
  if (descriptor.text) node.textContent = descriptor.text;
  applyAttrs(node, descriptor.attrs);
  for (const child of descriptor.children) {
    const childNode = renderNode(child, document);
    if (childNode) node.append(childNode);
  }
  // 表单控件的取值必须在子节点就位后再写入，否则 <select> 找不到对应选项。
  Object.assign(node, descriptor.props);
  Object.assign(node.dataset, descriptor.dataset);
  for (const [type, handler] of Object.entries(descriptor.on)) node.addEventListener(type, handler);
  return node;
}

export function renderInto(container, descriptor, document = globalThis.document) {
  const node = renderNode(descriptor, document);
  container.replaceChildren(...(node ? [node] : []));
  return node;
}
