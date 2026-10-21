<script>
  import { onDestroy } from 'svelte';
  import { safeMediaURL, downloadPayload, field } from './lib/model.js';
  import Icon from './Icon.svelte';
  export let message;
  export let api;
  export let onplayed = () => {};
  let source = '';
  let loading = false;
  let error = '';
  let loaded = false;
  let disposed = false;
  let ownsURL = false;
  $: kind = message.message_type;
  $: link = message.previewSource || safeMediaURL(message.media_link);
  // Never autoplay or preload historical attachments. Download on demand.
  async function load() {
    error = '';
    loading = true;
    try {
      let url = link;
      if (!url) {
        if (!message.media)
          throw new Error(
            'Este mensaje antiguo no conserva los datos de descarga.',
          );
        const endpoint = {
          audio: 'audio',
          image: 'image',
          sticker: 'sticker',
          video: 'video',
          document: 'document',
        }[kind];
        const result = await api.request(
          `/chat/download${endpoint}`,
          downloadPayload(message.media),
        );
        const data = field(result, 'Data');
        // Download responses can also contain a document data URL. Turn it into
        // a Blob so active content cannot become an executable navigation URL.
        if (typeof data !== 'string' || !data.startsWith('data:'))
          throw new Error('El servidor no devolvió el archivo.');
        const response = await fetch(data);
        const blob = await response.blob();
        url = URL.createObjectURL(blob);
        ownsURL = true;
      }
      if (disposed) {
        if (ownsURL) URL.revokeObjectURL(url);
        return;
      }
      source = url;
      loaded = true;
    } catch (e) {
      if (!disposed) error = e.message;
    } finally {
      if (!disposed) loading = false;
    }
  }
  onDestroy(() => {
    disposed = true;
    if (ownsURL) URL.revokeObjectURL(source);
  });
</script>

<div class="message-media">
  {#if !loaded}
    <button class="media-load" onclick={load} disabled={loading}>
      <span class="media-icon"
        ><Icon name={kind === 'audio' ? 'mic' : 'download'} /></span
      >
      <span
        ><strong
          >{loading
            ? 'Descargando…'
            : kind === 'audio'
              ? 'Escuchar audio'
              : kind === 'document'
                ? message.filename
                : 'Abrir ' +
                  ({ image: 'foto', video: 'video', sticker: 'sticker' }[
                    kind
                  ] || 'archivo')}</strong
        ><small
          >{kind === 'audio'
            ? 'Nota de voz · clic para cargar'
            : 'Descargar desde WhatsApp'}</small
        ></span
      >
    </button>
  {:else if kind === 'audio'}
    <audio
      controls
      preload="metadata"
      src={source}
      onplay={() => onplayed(message)}><track kind="captions" /></audio
    >
  {:else if kind === 'image' || kind === 'sticker'}
    <a href={source} target="_blank" rel="noopener noreferrer"
      ><img
        src={source}
        alt={kind === 'sticker' ? 'Sticker' : message.text || 'Imagen adjunta'}
        loading="lazy"
      /></a
    >
  {:else if kind === 'video'}
    <video controls preload="metadata" src={source}
      ><track kind="captions" /></video
    >
  {:else}
    <a
      class="document-link"
      href={source}
      download={message.filename}
      target="_blank"
      rel="noopener noreferrer"
      ><Icon name="file" />{message.filename}<Icon
        name="download"
        size={16}
      /></a
    >
  {/if}
  {#if error}<small class="inline-error"
      >{error}
      <button class="text-button" onclick={load}>Reintentar</button></small
    >{/if}
</div>
