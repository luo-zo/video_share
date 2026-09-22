import { mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { createMemoryHistory, createRouter } from 'vue-router';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import LoginView from '../../src/views/LoginView.vue';
import { useAuthStore } from '../../src/stores/auth';

describe('LoginView', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('signs in and returns only to the validated internal destination', async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/login', component: LoginView },
        { path: '/video/:id', component: { template: '<div>detail</div>' } },
        { path: '/', component: { template: '<div>home</div>' } },
      ],
    });
    await router.push('/login?returnTo=/video/7%3Ffrom%3Dlogin');
    await router.isReady();
    const store = useAuthStore();
    vi.spyOn(store, 'signIn').mockResolvedValue({
      id: 1, username: 'milo', nickname: '小猫', created_at: '2026-09-07T00:00:00Z',
    });

    const wrapper = mount(LoginView, { global: { plugins: [router] } });
    await wrapper.get('input[name="username"]').setValue(' Milo ');
    await wrapper.get('input[name="password"]').setValue('secret');
    await wrapper.get('form').trigger('submit');

    await vi.waitFor(() => expect(router.currentRoute.value.fullPath).toBe('/video/7?from=login'));
    expect(store.signIn).toHaveBeenCalledWith({ username: ' Milo ', password: 'secret' });
  });

  it('shows migrated field validation without calling the API', async () => {
    const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/login', component: LoginView }] });
    await router.push('/login');
    await router.isReady();
    const store = useAuthStore();
    const signIn = vi.spyOn(store, 'signIn');
    const wrapper = mount(LoginView, { global: { plugins: [router] } });

    await wrapper.get('form').trigger('submit');

    expect(wrapper.text()).toContain('请输入用户名。');
    expect(wrapper.text()).toContain('请输入密码。');
    expect(signIn).not.toHaveBeenCalled();
  });

  it('focuses the route heading on entry and the actual invalid field after validation', async () => {
    const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/login', component: LoginView }] });
    await router.push('/login');
    await router.isReady();
    const wrapper = mount(LoginView, { attachTo: document.body, global: { plugins: [router] } });
    await vi.waitFor(() => expect(document.activeElement).toBe(wrapper.get('#login-title').element));

    await wrapper.get('input[name="username"]').setValue('milo');
    await wrapper.get('form').trigger('submit');
    expect(document.activeElement).toBe(wrapper.get('input[name="password"]').element);
    wrapper.unmount();
  });
});
