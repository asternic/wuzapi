<script>
  import { getContext } from 'svelte';
  import { avatarContextKey } from './lib/avatars.js';
  import Icon from './Icon.svelte';
  export let name = '';
  export let jid = '';
  export let priority = false;
  const avatars = getContext(avatarContextKey);
  let visible = false;
  $: picture = $avatars?.entries.get(jid)?.url || '';
  $: if (visible && $avatars?.enabled && jid) avatars.request(jid, priority);
  function observe(node) {
    if (typeof IntersectionObserver === 'undefined') {
      visible = true;
      return;
    }
    const observer = new IntersectionObserver(([entry]) => {
      visible = entry.isIntersecting;
    });
    observer.observe(node);
    return {
      destroy() {
        observer.disconnect();
      },
    };
  }

  export let group = false;
  export let small = false;
  $: hue = [...name].reduce((sum, c) => sum + c.charCodeAt(0), 0) % 5;
  $: initials = name
    .trim()
    .split(/\s+/)
    .slice(0, 2)
    .map((w) => w[0] || '')
    .join('')
    .toUpperCase();
</script>

<span use:observe class="avatar tone-{hue}" class:small
  >{#if picture}<img
      src={picture}
      alt=""
      referrerpolicy="no-referrer"
      onerror={() => avatars.unavailable(jid, picture)}
    />{:else if group}<Icon
      name="users"
      size={small ? 17 : 22}
    />{:else}{initials.slice(0, 2) || '?'}{/if}</span
>
