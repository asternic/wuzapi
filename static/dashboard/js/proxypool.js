// Proxy Pool management (admin only)

let proxyPoolEntries = [];
let proxyPoolRendered = null; // JSON of the entries currently in the table
let proxyPoolEditingId = null;
let proxyPoolDeletingId = null;

document.addEventListener('DOMContentLoaded', function() {
  $('#proxyPoolEnabledToggle').checkbox();

  $('#addProxyPoolButton').on('click', function() {
    openProxyPoolModal(null);
  });

  $('#modalDeleteProxyPoolEntry').modal({
    closable: true,
    onApprove: function() {
      deleteProxyPoolEntry(proxyPoolDeletingId);
    }
  });
});

// Errors are shown inline (in the form or above the pool table), not as toasts.
function setProxyPoolError(selector, message) {
  if (message) {
    $(selector).text(message).removeClass('hidden');
  } else {
    $(selector).text('').addClass('hidden');
  }
}

async function proxyPoolRequest(method, path, body) {
  const admintoken = getLocalStorageItem('admintoken');
  const myHeaders = new Headers();
  myHeaders.append('authorization', admintoken);
  myHeaders.append('Content-Type', 'application/json');
  const res = await fetch(baseUrl + path, {
    method: method,
    headers: myHeaders,
    body: body ? JSON.stringify(body) : undefined
  });
  return res.json();
}

async function loadProxyPool() {
  try {
    const result = await proxyPoolRequest('GET', '/admin/proxy-pool');
    if (result.success === true) {
      proxyPoolEntries = result.data || [];
      // The list refreshes every few seconds; only rebuild the table when the
      // data changed so buttons are not replaced under the admin's cursor.
      const snapshot = JSON.stringify(proxyPoolEntries);
      if (snapshot !== proxyPoolRendered) {
        proxyPoolRendered = snapshot;
        renderProxyPool();
      }
    }
  } catch (error) {
    console.error('Failed to load proxy pool:', error);
  }
}

function getProxyPoolEntry(id) {
  return proxyPoolEntries.find(entry => entry.id === id) || null;
}

function proxyPoolEntryName(id) {
  const entry = getProxyPoolEntry(id);
  return entry ? (entry.label || entry.proxy_url) : 'pool';
}

function renderProxyPool() {
  // Instance rows and cards may render before the pool is loaded; fill in names.
  $('.proxy-pool-name').each(function() {
    $(this).text(proxyPoolEntryName($(this).data('proxy-pool-id')));
  });

  const tableBody = $('#proxy-pool-body');
  tableBody.empty();

  if (proxyPoolEntries.length === 0) {
    tableBody.append('<tr><td style="text-align:center;" colspan=5>No proxies in the pool. Instances connect with their own proxy or directly, as before.</td></tr>');
    return;
  }

  proxyPoolEntries.forEach((entry) => {
    const percent = Math.min(100, Math.round(entry.assigned_count / entry.max_devices * 100));
    const full = entry.assigned_count >= entry.max_devices;
    const row = `
      <tr id="proxy-pool-row-${entry.id}">
        <td>${escapeHtml(entry.label || '-')}</td>
        <td style="word-break: break-all;"><code>${escapeHtml(entry.proxy_url)}</code></td>
        <td style="min-width: 140px;">
          <div class="ui tiny ${full ? 'orange' : 'teal'} progress" data-percent="${percent}" style="margin: 0 0 0.3em 0;">
            <div class="bar" style="width: ${percent}%; min-width: 0;"></div>
          </div>
          ${entry.assigned_count} / ${entry.max_devices}${full ? ' (full)' : ''}
        </td>
        <td>
          <div class="ui ${entry.enabled ? 'green' : 'grey'} horizontal label">${entry.enabled ? 'Enabled' : 'Disabled'}</div>
        </td>
        <td>
          <button class="ui primary button dashboard-button" onclick="openProxyPoolModal('${entry.id}')">
            <i class="edit icon"></i> Edit
          </button>
          <button class="ui ${entry.enabled ? 'grey' : 'green'} button dashboard-button" onclick="toggleProxyPoolEntry('${entry.id}')">
            <i class="${entry.enabled ? 'pause' : 'play'} icon"></i> ${entry.enabled ? 'Disable' : 'Enable'}
          </button>
          <button class="ui negative button dashboard-button" onclick="confirmDeleteProxyPoolEntry('${entry.id}')" ${entry.assigned_count > 0 ? 'disabled title="Release the assigned instances first"' : ''}>
            <i class="trash alternate icon"></i> Delete
          </button>
        </td>
      </tr>
    `;
    tableBody.append(row);
  });
}

function openProxyPoolModal(id) {
  const entry = id ? getProxyPoolEntry(id) : null;
  proxyPoolEditingId = entry ? entry.id : null;

  setProxyPoolError('#proxyPoolFormError', '');
  $('#proxyPoolModalTitle').text(entry ? 'Edit Proxy' : 'Add Proxy');
  $('#proxyPoolLabel').val(entry ? entry.label : '');
  // The API never returns the proxy password, so editing starts from an empty
  // field and the URL is only sent when the admin types a new one.
  $('#proxyPoolUrl').val('');
  $('#proxyPoolUrl').attr('placeholder', entry ? entry.proxy_url : 'socks5://user:pass@203.0.113.10:1080 or http://203.0.113.10:3128');
  $('#proxyPoolUrlHelp').text(entry
    ? 'Leave empty to keep the current URL. A new URL is applied to assigned instances on their next connect.'
    : 'Only http:// and socks5:// proxies are supported.');
  $('#proxyPoolUrl').closest('.field').toggleClass('required', !entry);
  $('#proxyPoolMaxDevices').val(entry ? entry.max_devices : 1);
  $('#proxyPoolMaxDevices').attr('min', entry ? Math.max(1, entry.assigned_count) : 1);
  $('#proxyPoolEnabledToggle').checkbox(entry && !entry.enabled ? 'uncheck' : 'check');

  $('#modalProxyPoolEntry').modal({
    onApprove: function() {
      saveProxyPoolEntry();
      return false;
    }
  }).modal('show');
}

async function saveProxyPoolEntry() {
  const label = $('#proxyPoolLabel').val().trim();
  const proxyUrl = $('#proxyPoolUrl').val().trim();
  const maxDevices = parseInt($('#proxyPoolMaxDevices').val(), 10);
  const enabled = $('#proxyPoolEnabled').is(':checked');

  if (!proxyPoolEditingId && !proxyUrl) {
    setProxyPoolError('#proxyPoolFormError', 'Proxy URL is required');
    return;
  }
  if (proxyUrl && !/^(http|socks5):\/\/.+/.test(proxyUrl)) {
    setProxyPoolError('#proxyPoolFormError', 'Proxy URL must start with http:// or socks5://');
    return;
  }
  if (!Number.isInteger(maxDevices) || maxDevices < 1) {
    setProxyPoolError('#proxyPoolFormError', 'Max devices must be at least 1');
    return;
  }

  const payload = { label: label, max_devices: maxDevices, enabled: enabled };
  if (proxyUrl) {
    payload.proxy_url = proxyUrl;
  }

  try {
    const result = proxyPoolEditingId
      ? await proxyPoolRequest('PUT', '/admin/proxy-pool/' + proxyPoolEditingId, payload)
      : await proxyPoolRequest('POST', '/admin/proxy-pool', payload);
    if (result.success === true) {
      $('#modalProxyPoolEntry').modal('hide');
      setProxyPoolError('#proxyPoolError', '');
      loadProxyPool();
    } else {
      setProxyPoolError('#proxyPoolFormError', result.error || 'Failed to save proxy');
    }
  } catch (error) {
    setProxyPoolError('#proxyPoolFormError', 'Failed to save proxy');
    console.error('Proxy pool save error:', error);
  }
}

async function toggleProxyPoolEntry(id) {
  const entry = getProxyPoolEntry(id);
  if (!entry) return;
  try {
    const result = await proxyPoolRequest('PUT', '/admin/proxy-pool/' + id, { enabled: !entry.enabled });
    if (result.success === true) {
      setProxyPoolError('#proxyPoolError', '');
      loadProxyPool();
    } else {
      setProxyPoolError('#proxyPoolError', result.error || 'Failed to update proxy');
    }
  } catch (error) {
    setProxyPoolError('#proxyPoolError', 'Failed to update proxy');
  }
}

function confirmDeleteProxyPoolEntry(id) {
  const entry = getProxyPoolEntry(id);
  if (!entry) return;
  proxyPoolDeletingId = id;
  $('#deleteProxyPoolLabel').text(entry.label || entry.proxy_url);
  $('#modalDeleteProxyPoolEntry').modal('show');
}

async function deleteProxyPoolEntry(id) {
  if (!id) return;
  try {
    const result = await proxyPoolRequest('DELETE', '/admin/proxy-pool/' + id);
    if (result.success === true) {
      setProxyPoolError('#proxyPoolError', '');
      loadProxyPool();
    } else {
      setProxyPoolError('#proxyPoolError', result.error || 'Failed to delete proxy');
    }
  } catch (error) {
    setProxyPoolError('#proxyPoolError', 'Failed to delete proxy');
  }
  proxyPoolDeletingId = null;
}

async function releaseProxyPool(userId) {
  try {
    const result = await proxyPoolRequest('POST', '/admin/users/' + userId + '/proxy-pool/release');
    if (result.success === true) {
      loadProxyPool();
      updateAdmin();
    } else {
      console.error('Failed to release proxy:', result.error);
    }
  } catch (error) {
    console.error('Failed to release proxy:', error);
  }
}

// Short description of an instance's proxy for the instances table and card.
function instanceProxyLabel(instance) {
  if (instance.proxy_pool_id) {
    const name = proxyPoolEntryName(instance.proxy_pool_id);
    return `<div class="ui teal horizontal label" title="Assigned from the proxy pool"><i class="sitemap icon"></i><span class="proxy-pool-name" data-proxy-pool-id="${instance.proxy_pool_id}">${escapeHtml(name)}</span></div>`;
  }
  if (instance.proxy_config && instance.proxy_config.enabled) {
    return '<div class="ui blue horizontal label" title="Proxy set on the instance">Custom</div>';
  }
  return '<span style="color: #888;">None</span>';
}
