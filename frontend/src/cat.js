// The cat stays decorative: form controls work independently of its animation.
export function initCat() {
  const cat = document.getElementById('cat-character');
  const pupils = document.getElementById('cat-pupils');
  const password = document.getElementById('password');
  const toggle = document.getElementById('toggle-password');
  const caption = document.getElementById('cat-caption');
  if (!cat || !pupils || !password || !toggle || !caption) return;

  const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)');
  let frame = 0;
  let pointer = null;

  function setCovering() {
    const passwordActive = document.activeElement === password || document.activeElement === toggle;
    const covering = passwordActive || (password.type === 'text' && password.value.length > 0);
    cat.classList.toggle('is-covering', covering);
    caption.textContent = covering ? '你安心输入，小黑保证不偷看。' : '小黑已就位，等你好久了。';
  }

  function drawGaze() {
    frame = 0;
    if (!pointer || reducedMotion.matches || cat.classList.contains('is-covering')) return;
    const bounds = cat.getBoundingClientRect();
    const x = Math.max(-7, Math.min(7, (pointer.x - bounds.left - bounds.width / 2) / 65));
    const y = Math.max(-3, Math.min(3, (pointer.y - bounds.top - bounds.height / 3) / 90));
    pupils.style.setProperty('--look-x', `${x}px`);
    pupils.style.setProperty('--look-y', `${y}px`);
  }

  document.addEventListener('pointermove', (event) => {
    if (event.pointerType !== 'mouse' || reducedMotion.matches) return;
    pointer = { x: event.clientX, y: event.clientY };
    if (!frame) frame = requestAnimationFrame(drawGaze);
  }, { passive: true });
  document.addEventListener('pointerdown', (event) => {
    const active = document.activeElement;
    if ((active === password || active === toggle) &&
        event.target instanceof Element && !event.target.closest('#password-field')) {
      active.blur();
      cat.classList.remove('is-covering');
      caption.textContent = '小黑已就位，等你好久了。';
    }
  }, { passive: true });
  document.documentElement.addEventListener('pointerleave', () => {
    pointer = null;
    pupils.style.setProperty('--look-x', '0px');
    pupils.style.setProperty('--look-y', '0px');
  });
  document.addEventListener('focusin', setCovering);
  document.addEventListener('focusout', () => requestAnimationFrame(setCovering));
  password.addEventListener('input', setCovering);
  toggle.addEventListener('click', setCovering);
  window.addEventListener('blur', () => {
    pupils.style.setProperty('--look-x', '0px');
    pupils.style.setProperty('--look-y', '0px');
  });
  setCovering();
}
