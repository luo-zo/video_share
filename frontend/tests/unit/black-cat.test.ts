import { mount } from '@vue/test-utils';
import { describe, expect, it, vi } from 'vitest';

import BlackCat from '../../src/components/BlackCat.vue';

describe('BlackCat', () => {
  it('removes the pointer listener when its view unmounts', () => {
    const add = vi.spyOn(window, 'addEventListener');
    const remove = vi.spyOn(window, 'removeEventListener');

    const wrapper = mount(BlackCat);
    const pointerRegistration = add.mock.calls.find(([type]) => type === 'pointermove');
    expect(pointerRegistration).toBeTruthy();

    wrapper.unmount();
    expect(remove).toHaveBeenCalledWith('pointermove', pointerRegistration?.[1]);
  });
});
