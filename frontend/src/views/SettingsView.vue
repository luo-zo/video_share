<script setup lang="ts">
import { reactive, ref } from 'vue';
import { AuthError } from '../api/auth';
import { useAuthStore } from '../stores/auth';

const auth = useAuthStore();
const profile = reactive({ nickname: auth.user?.nickname ?? '', bio: auth.user?.bio ?? '' });
const password = reactive({ oldPassword: '', newPassword: '' });
const profileBusy = ref(false);
const passwordBusy = ref(false);
const message = ref('');
const error = ref('');

async function saveProfile(): Promise<void> {
  message.value = ''; error.value = ''; profileBusy.value = true;
  try {
    const user = await auth.updateProfile(profile);
    profile.nickname = user.nickname; profile.bio = user.bio ?? '';
    message.value = '资料已保存。';
  } catch (caught) { error.value = caught instanceof AuthError ? caught.message : '资料保存失败。'; }
  finally { profileBusy.value = false; }
}

async function changePassword(): Promise<void> {
  message.value = ''; error.value = ''; passwordBusy.value = true;
  try {
    await auth.changePassword(password);
    password.oldPassword = ''; password.newPassword = '';
    message.value = '密码已修改，请重新登录。';
  } catch (caught) { error.value = caught instanceof AuthError ? caught.message : '密码修改失败。'; }
  finally { passwordBusy.value = false; }
}
</script>

<template>
  <div class="app-content">
    <section class="app-panel" aria-labelledby="settings-title">
      <div class="panel-heading"><div><span class="panel-kicker">ACCOUNT / SETTINGS</span><h2 id="settings-title">账号设置</h2><p>资料更新后会立即同步到你的公开身份。</p></div></div>
      <p v-if="message" class="app-message" role="status">{{ message }}</p>
      <p v-if="error" class="app-message" data-kind="error" role="alert">{{ error }}</p>
      <form class="settings-form" @submit.prevent="saveProfile">
        <label>用户名<input :value="auth.user?.username" disabled autocomplete="username"></label>
        <label>昵称<input v-model="profile.nickname" maxlength="64" autocomplete="nickname"></label>
        <label>简介<textarea v-model="profile.bio" maxlength="200" rows="4"></textarea></label>
        <button class="submit-button" type="submit" :disabled="profileBusy">保存资料</button>
      </form>
      <form class="settings-form" @submit.prevent="changePassword">
        <h3>修改密码</h3>
        <label>旧密码<input v-model="password.oldPassword" type="password" autocomplete="current-password"></label>
        <label>新密码<input v-model="password.newPassword" type="password" autocomplete="new-password"></label>
        <button class="submit-button" type="submit" :disabled="passwordBusy">修改并退出其他会话</button>
      </form>
    </section>
  </div>
</template>
