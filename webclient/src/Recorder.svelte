<script>
  import { onDestroy } from 'svelte';
  import Icon from './Icon.svelte';
  export let onsend;
  export let oncancel;
  let recorder;
  let stream;
  let timer;
  let seconds = 0;
  let error = '';
  let state = 'ready';
  let blob;
  let preview = '';
  let alive = true;
  let chunks = [];
  function stopTracks() {
    stream?.getTracks().forEach((track) => track.stop());
    clearInterval(timer);
  }
  async function start() {
    error = '';
    try {
      if (
        !navigator.mediaDevices?.getUserMedia ||
        typeof MediaRecorder === 'undefined'
      )
        throw new Error(
          'La grabación requiere HTTPS o localhost y un navegador compatible.',
        );
      const acquired = await navigator.mediaDevices.getUserMedia({
        audio: true,
      });
      if (!alive) {
        acquired.getTracks().forEach((track) => track.stop());
        return;
      }
      stream = acquired;
      const mimeType = [
        'audio/ogg;codecs=opus',
        'audio/webm;codecs=opus',
        'audio/mp4',
      ].find((type) => MediaRecorder.isTypeSupported(type));
      recorder = new MediaRecorder(stream, mimeType ? { mimeType } : {});
      chunks = [];
      seconds = 0;
      recorder.ondataavailable = (event) => {
        if (event.data.size) chunks.push(event.data);
      };
      recorder.onstop = () => {
        stopTracks();
        if (!alive) return;
        blob = new Blob(chunks, { type: recorder.mimeType || 'audio/webm' });
        if (blob.size > 16 * 1024 * 1024) {
          error = 'La grabación supera 16 MiB.';
          state = 'ready';
          return;
        }
        preview = URL.createObjectURL(blob);
        state = 'preview';
      };
      recorder.onerror = () => {
        stopTracks();
        error = 'No se pudo grabar el audio.';
        state = 'ready';
      };
      recorder.start(1000);
      state = 'recording';
      timer = setInterval(() => {
        seconds += 1;
        if (seconds >= 300) stop();
      }, 1000);
    } catch (e) {
      error = e.message;
      stopTracks();
    }
  }
  function stop() {
    if (recorder?.state === 'recording') recorder.stop();
  }
  async function send() {
    state = 'sending';
    error = '';
    try {
      await onsend(blob, Math.max(1, seconds));
    } catch (e) {
      if (alive) {
        error = e.message;
        state = 'preview';
      }
    }
  }
  onDestroy(() => {
    alive = false;
    stop();
    stopTracks();
    if (preview) URL.revokeObjectURL(preview);
  });
</script>

<div class="recorder">
  <button
    class="icon-button"
    title="Cancelar grabación"
    onclick={oncancel}
    disabled={state === 'sending'}><Icon name="close" /></button
  >
  {#if state === 'ready'}
    <span class="recorder-label"
      >Nota de voz <small>Hasta 5 minutos</small></span
    >
    <button class="primary" onclick={start}
      ><Icon name="mic" size={17} />Grabar</button
    >
  {:else if state === 'recording'}
    <span class="recording-dot"></span><strong
      >{Math.floor(seconds / 60)}:{String(seconds % 60).padStart(
        2,
        '0',
      )}</strong
    ><span class="recorder-label">Grabando…</span>
    <button class="primary" onclick={stop}
      ><Icon name="stop" size={17} />Terminar</button
    >
  {:else}
    <audio controls src={preview}><track kind="captions" /></audio>
    <button
      class="send-button"
      title="Enviar nota de voz"
      onclick={send}
      disabled={state === 'sending'}><Icon name="send" /></button
    >
  {/if}
  {#if error}<small class="inline-error">{error}</small>{/if}
</div>
