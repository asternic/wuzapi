<script>
  export let name;
  export let size = 20;
  const paths = {
    chat: 'M21 11.5a8.4 8.4 0 0 1-.9 3.8 8.5 8.5 0 0 1-7.6 4.7 8.4 8.4 0 0 1-3.8-.9L3 21l1.9-5.7a8.4 8.4 0 0 1-.9-3.8 8.5 8.5 0 0 1 4.7-7.6 8.4 8.4 0 0 1 3.8-.9h.5a8.5 8.5 0 0 1 8 8v.5Z',
    users:
      'M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2m20 0v-2a4 4 0 0 0-3-3.87M16 3.13a4 4 0 0 1 0 7.75M13 7a4 4 0 1 1-8 0 4 4 0 0 1 8 0Z',
    person: 'M20 21v-2a7 7 0 0 0-14 0v2M17 7a4 4 0 1 1-8 0 4 4 0 0 1 8 0Z',
    search: 'm21 21-4.3-4.3M19 11a8 8 0 1 1-16 0 8 8 0 0 1 16 0Z',
    plus: 'M12 5v14M5 12h14',
    send: 'm22 2-7 20-4-9-9-4 20-7ZM22 2 11 13',
    mic: 'M12 2a3 3 0 0 0-3 3v7a3 3 0 0 0 6 0V5a3 3 0 0 0-3-3ZM5 10v2a7 7 0 0 0 14 0v-2M12 19v3M8 22h8',
    clip: 'm21.4 11.6-9.2 9.2a6 6 0 0 1-8.5-8.5L13.6 2.4a4 4 0 0 1 5.7 5.7L9.4 18a2 2 0 0 1-2.8-2.8l9.2-9.2',
    settings:
      'M12 8a4 4 0 1 1 0 8 4 4 0 0 1 0-8ZM12 2v3m0 14v3M2 12h3m14 0h3M5 5l2 2m10 10 2 2M5 19l2-2M17 7l2-2',
    close: 'm6 6 12 12M6 18 18 6',
    back: 'm15 18-6-6 6-6',
    refresh:
      'M20 7v5h-5M4 17v-5h5M6.1 6.1A8 8 0 0 1 19.9 9M4.1 15a8 8 0 0 0 13.8 2.9',
    history: 'M3 12a9 9 0 1 0 2.6-6.4L3 8m0-5v5h5m4-1v5l3 2',
    download: 'M12 3v12m-5-5 5 5 5-5M5 17v4h14v-4',
    file: 'M14 2H6v20h12V6l-4-4ZM14 2v5h5M9 13h6M9 17h6',
    check: 'm5 12 4 4L19 6',
    info: 'M12 11v6m0-10v.01M22 12a10 10 0 1 1-20 0 10 10 0 0 1 20 0Z',
    stop: 'M5 5h14v14H5Z',
    logout: 'M9 21H3V3h6m7 14 5-5-5-5m5 5H9',
    lock: 'M5 10h14v12H5ZM8 10V6a4 4 0 0 1 8 0v4',
    more: 'M12 5v.01M12 12v.01M12 19v.01',
    arrow: 'M5 12h14m-6-6 6 6-6 6',
    sun: 'M12 2v2m0 16v2M2 12h2m16 0h2M5 5l1 1m12 12 1 1M5 19l1-1M18 6l1-1M16 12a4 4 0 1 1-8 0 4 4 0 0 1 8 0Z',
  };
</script>

<svg
  width={size}
  height={size}
  viewBox="0 0 24 24"
  fill="none"
  stroke="currentColor"
  stroke-width="1.7"
  stroke-linecap="round"
  stroke-linejoin="round"
  aria-hidden="true"><path d={paths[name] || paths.chat} /></svg
>
