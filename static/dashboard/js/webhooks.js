// Webhooks Management Module

/**
 * Obter configurações de webhook da instância atual
 * @returns {Promise<object|null>} Configurações do webhook ou null
 */
async function getWebhook() {
    if (!instancesState?.currentInstance) return null;
    
    try {
        addLog('Obtendo configurações de webhook...');
        const response = await makeApiCall(`/instances/${instancesState.currentInstance}/webhook`, 'GET');
        
        const webhookUrl = document.getElementById('webhookUrl');
        const webhookUrlInput = document.getElementById('webhookUrlInput');
        const subscribedEvents = document.getElementById('subscribedEvents');
        
        if (response.success && response.data) {
            if (response.data.url) {
                if (webhookUrl) webhookUrl.textContent = response.data.url;
                if (webhookUrlInput) webhookUrlInput.value = response.data.url;
            } else {
                if (webhookUrl) webhookUrl.textContent = '-';
                if (webhookUrlInput) webhookUrlInput.value = '';
            }
            
            if (response.data.events) {
                const events = Array.isArray(response.data.events) 
                    ? response.data.events 
                    : [response.data.events];
                    
                if (subscribedEvents) subscribedEvents.textContent = events.join(', ');
                
                // Marcar checkboxes
                document.querySelectorAll('input[type=checkbox][id^=event]').forEach(checkbox => {
                    checkbox.checked = events.includes(checkbox.value);
                });
            }
            
            // Atualizar checkbox ativo
            const webhookActiveCheck = document.getElementById('webhookActiveCheck');
            if (webhookActiveCheck) {
                webhookActiveCheck.checked = response.data.enabled !== false;
            }
            
            addLog('Configurações de webhook obtidas com sucesso', 'success');
            return response.data;
        } else {
            // Tratar especificamente o caso de webhook não configurado
            if (response.error && (
                response.error.includes('não configurado') || 
                response.error.includes('not configured') ||
                response.status === 404
            )) {
                addLog('Sem webhook configurado', 'info');
                // Limpar campos
                if (webhookUrl) webhookUrl.textContent = '-';
                if (webhookUrlInput) webhookUrlInput.value = '';
                if (subscribedEvents) subscribedEvents.textContent = '-';
                
                // Desmarcar checkbox ativo
                const webhookActiveCheck = document.getElementById('webhookActiveCheck');
                if (webhookActiveCheck) {
                    webhookActiveCheck.checked = false;
                }
                
                // Desmarcar todos os eventos
                document.querySelectorAll('input[type=checkbox][id^=event]').forEach(checkbox => {
                    checkbox.checked = false;
                });
                
                return null;
            }
            
            // Outros erros
            addLog(`Falha ao obter webhook: ${response.error || 'Erro desconhecido'}`, 'error');
            return null;
        }
    } catch (error) {
        addLog(`Erro ao obter configurações de webhook: ${error.message || 'Erro desconhecido'}`, 'error');
        return null;
    }
}

/**
 * Salvar configurações de webhook
 * @returns {Promise<boolean>} Sucesso da operação
 */
async function saveWebhook() {
    if (!instancesState?.currentInstance) {
        showToast('Selecione uma instância primeiro', 'warning');
        return false;
    }
    
    const webhookUrlInput = document.getElementById('webhookUrlInput');
    const webhookActiveCheck = document.getElementById('webhookActiveCheck');
    
    if (!webhookUrlInput || !webhookActiveCheck) return false;
    
    const url = webhookUrlInput.value.trim();
    const active = webhookActiveCheck.checked;
    
    if (!url && active) {
        addLog('URL do webhook não informada', 'error');
        const webhookErrorAlert = document.getElementById('webhookErrorAlert');
        if (webhookErrorAlert) {
            webhookErrorAlert.textContent = 'Por favor, informe a URL do webhook.';
            webhookErrorAlert.classList.remove('d-none');
            setTimeout(() => webhookErrorAlert.classList.add('d-none'), 5000);
        }
        return false;
    }
    
    // Obter eventos selecionados
    const events = [];
    document.querySelectorAll('input[type=checkbox][id^=event]:checked').forEach(checkbox => {
        events.push(checkbox.value);
    });
    
    try {
        showLoading('Salvando configurações...');
        addLog('Salvando configurações de webhook...');
        
        let response;
        
        if (!url || !active) {
            // Remover webhook
            response = await makeApiCall(`/instances/${instancesState.currentInstance}/webhook/delete`, 'DELETE');
        } else {
            // Configurar webhook
            response = await makeApiCall(`/instances/${instancesState.currentInstance}/webhook`, 'POST', {
                webhook: url,
                events: events
            });
        }
        
        hideLoading();
        
        const webhookUrl = document.getElementById('webhookUrl');
        const subscribedEvents = document.getElementById('subscribedEvents');
        const webhookSavedAlert = document.getElementById('webhookSavedAlert');
        const webhookErrorAlert = document.getElementById('webhookErrorAlert');
        
        if (response.success) {
            if (!url || !active) {
                addLog('Webhook removido com sucesso', 'success');
                showToast('Webhook removido com sucesso');
                if (webhookUrl) webhookUrl.textContent = '-';
                if (subscribedEvents) subscribedEvents.textContent = '-';
            } else {
                addLog('Webhook configurado com sucesso', 'success');
                showToast('Webhook configurado com sucesso');
                if (webhookUrl) webhookUrl.textContent = url;
                if (subscribedEvents) subscribedEvents.textContent = events.join(', ');
            }
            
            if (webhookSavedAlert) {
                webhookSavedAlert.classList.remove('d-none');
                setTimeout(() => webhookSavedAlert.classList.add('d-none'), 5000);
            }
            
            return true;
        } else {
            addLog(`Falha ao configurar webhook: ${response.error}`, 'error');
            showToast(`Falha ao configurar webhook: ${response.error}`, 'danger');
            
            if (webhookErrorAlert) {
                webhookErrorAlert.textContent = response.error || 'Erro ao configurar webhook.';
                webhookErrorAlert.classList.remove('d-none');
                setTimeout(() => webhookErrorAlert.classList.add('d-none'), 5000);
            }
            
            return false;
        }
    } catch (error) {
        hideLoading();
        addLog('Erro ao salvar configurações de webhook', 'error');
        showToast('Erro ao salvar configurações de webhook', 'danger');
        
        const webhookErrorAlert = document.getElementById('webhookErrorAlert');
        if (webhookErrorAlert) {
            webhookErrorAlert.textContent = 'Erro ao conectar com o servidor.';
            webhookErrorAlert.classList.remove('d-none');
            setTimeout(() => webhookErrorAlert.classList.add('d-none'), 5000);
        }
        
        return false;
    }
}

/**
 * Testar webhook configurado
 * @returns {Promise<boolean>} Sucesso da operação
 */
async function testWebhook() {
    if (!instancesState?.currentInstance) {
        showToast('Selecione uma instância primeiro', 'warning');
        return false;
    }
    
    try {
        showLoading('Testando webhook...');
        addLog('Enviando requisição de teste para o webhook...');
        
        const response = await makeApiCall(`/instances/${instancesState.currentInstance}/webhook/test`, 'POST');
        
        hideLoading();
        
        if (response.success) {
            addLog('Webhook testado com sucesso', 'success');
            showToast('Webhook testado com sucesso');
            return true;
        } else {
            addLog(`Falha ao testar webhook: ${response.error || 'Erro desconhecido'}`, 'error');
            showToast(`Falha ao testar webhook: ${response.error || 'Erro desconhecido'}`, 'danger');
            return false;
        }
    } catch (error) {
        hideLoading();
        addLog('Erro ao testar webhook', 'error');
        showToast('Erro ao testar webhook', 'danger');
        return false;
    }
}

/**
 * Remover webhook configurado
 * @returns {Promise<boolean>} Sucesso da operação
 */
async function deleteWebhook() {
    if (!instancesState?.currentInstance) {
        showToast('Selecione uma instância primeiro', 'warning');
        return false;
    }
    
    try {
        showLoading('Removendo webhook...');
        addLog('Removendo configuração de webhook...');
        
        const response = await makeApiCall(`/instances/${instancesState.currentInstance}/webhook/delete`, 'DELETE');
        
        hideLoading();
        
        const webhookUrl = document.getElementById('webhookUrl');
        const webhookUrlInput = document.getElementById('webhookUrlInput');
        const subscribedEvents = document.getElementById('subscribedEvents');
        
        if (response.success) {
            addLog('Webhook removido com sucesso', 'success');
            showToast('Webhook removido com sucesso');
            
            // Limpar campos
            if (webhookUrl) webhookUrl.textContent = '-';
            if (webhookUrlInput) webhookUrlInput.value = '';
            if (subscribedEvents) subscribedEvents.textContent = '-';
            
            // Desmarcar checkboxes
            document.querySelectorAll('input[type=checkbox][id^=event]').forEach(checkbox => {
                checkbox.checked = false;
            });
            
            return true;
        } else {
            addLog(`Falha ao remover webhook: ${response.error || 'Erro desconhecido'}`, 'error');
            showToast(`Falha ao remover webhook: ${response.error || 'Erro desconhecido'}`, 'danger');
            return false;
        }
    } catch (error) {
        hideLoading();
        addLog('Erro ao remover webhook', 'error');
        showToast('Erro ao remover webhook', 'danger');
        return false;
    }
}

// Inicialização
document.addEventListener('DOMContentLoaded', () => {
    // Registrar evento do botão salvar webhook
    const saveWebhookBtn = document.getElementById('saveWebhookBtn');
    if (saveWebhookBtn) {
        saveWebhookBtn.addEventListener('click', saveWebhook);
    }
    
    // Registrar evento do botão testar webhook
    const testWebhookBtn = document.getElementById('testWebhookBtn');
    if (testWebhookBtn) {
        testWebhookBtn.addEventListener('click', testWebhook);
    }
    
    // Registrar evento do botão remover webhook
    const deleteWebhookBtn = document.getElementById('deleteWebhookBtn');
    if (deleteWebhookBtn) {
        deleteWebhookBtn.addEventListener('click', deleteWebhook);
    }
    
    // Event listener para o checkbox "Todos os eventos"
    const eventAllCheckbox = document.getElementById('eventAll');
    if (eventAllCheckbox) {
        eventAllCheckbox.addEventListener('change', (e) => {
            const isChecked = e.target.checked;
            
            // Marcar ou desmarcar todos os outros checkboxes
            document.querySelectorAll('input[type=checkbox][id^=event]').forEach(checkbox => {
                if (checkbox.id !== 'eventAll') {
                    checkbox.checked = isChecked;
                    checkbox.disabled = isChecked;
                }
            });
        });
    }
}); 