// Instances Management Module

// Estado global das instâncias
let instancesState = {
    instances: [],
    currentInstance: null,
    qrCheckInterval: null
};

/**
 * Carregar instâncias disponíveis para o usuário
 */
async function loadInstances() {
    try {
        // Limpar lista atual
        instancesState.instances = [];
        
        // Limpar UI
        const instancesList = document.getElementById('instancesList');
        if (instancesList) {
            instancesList.innerHTML = '<div class="text-center py-3"><div class="spinner-border spinner-border-sm text-primary" role="status"></div> Carregando instâncias...</div>';
        }
        
        // Carregar instâncias do servidor
        const response = await makeApiCall('/instances', 'GET');
        
        // Verificar resposta
        if (response.success && response.data) {
            // Atualizar estado de instâncias local
            if (Array.isArray(response.data)) {
                instancesState.instances = response.data;
                
                // Atualizar UI
                updateInstancesList();
                
                // Verificar tipo de usuário para UI específica
                const isAdmin = window.authState?.isAdmin === true;
                
                if (isAdmin) {
                    // Admin pode ver todas as instâncias
                    showAdminUI();
                    
                    // Restaurar seleção anterior
                    const savedInstance = localStorage.getItem('currentInstance');
                    
                    if (savedInstance) {
                        // Encontrar a instância na lista
                        const instance = instancesState.instances.find(i => String(i.id) === String(savedInstance));
                        
                        if (instance) {
                            // Selecionar instância com delay para evitar conflitos
                            setTimeout(() => {
                                selectInstance(savedInstance);
                            }, 300);
                        } else if (instancesState.instances.length > 0) {
                            // Se a instância salva não existir mais, selecionar a primeira
                            setTimeout(() => {
                                selectInstance(instancesState.instances[0].id);
                            }, 300);
                        }
                    } else if (instancesState.instances.length > 0) {
                        // Se admin não tiver seleção anterior, selecionar a primeira
                        setTimeout(() => {
                            selectInstance(instancesState.instances[0].id);
                        }, 300);
                    }
                } else {
                    // Usuário normal vê apenas sua instância
                    hideAdminUI();
                    
                    // Mostrar UI para usuário normal
                    document.querySelectorAll('.user-only').forEach(el => {
                        el.style.display = '';
                    });
                    
                    // Se houver instâncias, selecionar automaticamente a primeira
                    if (instancesState.instances.length > 0) {
                        setTimeout(() => {
                            selectInstance(instancesState.instances[0].id);
                        }, 300);
                    } else {
                        showNoInstancesMessage('Nenhuma instância disponível');
                    }
                }
                
                return true;
            } else {
                addLog('Formato de resposta das instâncias inválido', 'error');
                
                // Mostrar UI de erro
                if (instancesList) {
                    instancesList.innerHTML = '<div class="alert alert-danger">Erro ao carregar instâncias: formato inválido</div>';
                }
                
                return false;
            }
        } else {
            addLog(`Erro ao carregar instâncias: ${response.error || 'Erro desconhecido'}`, 'error');
            
            // Mostrar UI de erro
            if (instancesList) {
                instancesList.innerHTML = `<div class="alert alert-danger">Erro ao carregar instâncias: ${response.error || 'Erro desconhecido'}</div>`;
            }
            
            return false;
        }
    } catch (error) {
        addLog(`Erro ao carregar instâncias: ${error.message}`, 'error');
        
        // Mostrar UI de erro
        const instancesList = document.getElementById('instancesList');
        if (instancesList) {
            instancesList.innerHTML = `<div class="alert alert-danger">Erro ao carregar instâncias: ${error.message}</div>`;
        }
        
        return false;
    }
}

/**
 * Selecionar uma instância
 * @param {string|number} instanceId - ID da instância
 * @param {boolean} checkStatusNow - Se deve verificar o status imediatamente
 */
async function selectInstance(instanceId, checkStatusNow = true) {
    console.log(`[INSTANCE] Selecionando instância ${instanceId}`);
    
    if (!instanceId) {
        showToast('ID da instância inválido', 'warning');
        return;
    }
    
    try {
        // Encontrar a instância nos dados
        const instance = instancesState.instances.find(inst => String(inst.id) === String(instanceId));
        
        if (!instance) {
            console.error(`[INSTANCE] Instância ${instanceId} não encontrada`);
            showToast('Instância não encontrada', 'danger');
            return;
        }
        
        // Armazenar ID da instância atual
        instancesState.currentInstance = instanceId;
        
        // Salvar no localStorage para persistir
        localStorage.setItem('currentInstance', instanceId);
        
        // Atualizar UI
        updateUIForSelectedInstance(instance);
        
        // Iniciar verificação de status se solicitado
        if (checkStatusNow) {
            // Iniciar check status com um pequeno delay para evitar sobrecarga
            setTimeout(async () => {
                await checkStatus(instanceId);
                
                // Iniciar polling de status após verificação inicial
                startStatusPolling();
            }, 100);
        } else {
            // Verificar se há um intervalo existente
            if (!appState.statusPollingInterval) {
                startStatusPolling();
            }
        }
        
        // Mostrar dashboard
        showInstanceDashboard();
        
        return true;
    } catch (error) {
        console.error('[INSTANCE] Erro ao selecionar instância:', error);
        showToast('Erro ao selecionar instância', 'danger');
        return false;
    }
}

/**
 * Atualizar a UI para a instância selecionada
 * @param {Object} instance - Dados da instância
 */
function updateUIForSelectedInstance(instance) {
    // Destacar a instância selecionada na lista
    document.querySelectorAll('.instance-item').forEach(item => {
        item.classList.remove('active');
    });
    
    const selectedItem = document.querySelector(`.instance-item[data-instance-id="${instance.id}"]`);
    if (selectedItem) selectedItem.classList.add('active');
    
    // Atualizar nome da instância no título
    const currentInstanceName = document.getElementById('currentInstanceName');
    if (currentInstanceName) {
        currentInstanceName.textContent = instance.name || `Instância ${instance.id}`;
    }
    
    // Atualizar seção de detalhes
    updateInstanceDetailUI(instance);
    
    // Atualizar status da instância
    updateStatusUI(instance.isConnected, instance.isLoggedIn);
    
    // Atualizar visibilidade dos botões de ação
    if (typeof updateActionButtons === 'function') {
        updateActionButtons();
    }
    
    console.log(`[INSTANCE] Interface atualizada para instância ${instance.id}`);
}

/**
 * Mostrar UI específica para admin
 */
function showAdminUI() {
    // Mostrar elementos específicos para admin
    document.querySelectorAll('.admin-only').forEach(el => {
        el.style.display = '';
    });
    
    // Ocultar elementos específicos para usuário normal
    document.querySelectorAll('.user-only').forEach(el => {
        el.style.display = 'none';
    });
    
    // Mostrar painel de administração
    const adminDashboardPanel = document.getElementById('adminDashboardPanel');
    if (adminDashboardPanel) {
        adminDashboardPanel.style.display = '';
    }
    
    // Mostrar lista de instâncias admin
    const adminInstancesListPanel = document.getElementById('adminInstancesListPanel');
    if (adminInstancesListPanel) {
        adminInstancesListPanel.style.display = '';
    }
    
    // Atualizar estatísticas admin
    if (typeof loadAdminStats === 'function') {
        loadAdminStats();
    }
}

/**
 * Ocultar UI específica para admin
 */
function hideAdminUI() {
    // Ocultar elementos específicos para admin
    document.querySelectorAll('.admin-only').forEach(el => {
        el.style.display = 'none';
    });
    
    // Mostrar elementos específicos para usuário normal
    document.querySelectorAll('.user-only').forEach(el => {
        el.style.display = '';
    });
    
    // Ocultar painel de administração
    const adminDashboardPanel = document.getElementById('adminDashboardPanel');
    if (adminDashboardPanel) {
        adminDashboardPanel.style.display = 'none';
    }
    
    // Ocultar lista de instâncias admin
    const adminInstancesListPanel = document.getElementById('adminInstancesListPanel');
    if (adminInstancesListPanel) {
        adminInstancesListPanel.style.display = 'none';
    }
}

/**
 * Atualizar a lista de instâncias na UI
 */
function updateInstancesList() {
    const isAdmin = window.authState?.isAdmin === true;
    
    // Atualizar lista lateral (apenas para admin)
    if (isAdmin) {
        const instancesList = document.getElementById('instanceList');
        if (instancesList) {
            if (instancesState.instances.length > 0) {
                // Criar elementos para cada instância
                let html = '';
                instancesState.instances.forEach(instance => {
                    const isSelected = instance.id == instancesState.currentInstance;
                    html += `
                        <div class="instance-item ${isSelected ? 'active' : ''}" data-id="${instance.id}">
                            <div class="instance-status">
                                <div class="status-dot ${instance.connected ? 'connected' : 'disconnected'}"></div>
                            </div>
                            <div class="instance-info">
                                <div class="instance-name">${instance.name || `Instância ${instance.id}`}</div>
                                <div class="instance-details">
                                    ${instance.phone ? `<span>${instance.phone}</span>` : '<span class="text-muted">Não conectada</span>'}
                                </div>
                            </div>
                        </div>
                    `;
                });
                instancesList.innerHTML = html;
                
                // Adicionar eventos de clique
                document.querySelectorAll('.instance-item').forEach(item => {
                    item.addEventListener('click', function() {
                        const id = this.getAttribute('data-id');
                        selectInstance(id);
                    });
                });
                
                // Ocultar mensagem de "sem instâncias"
                const noInstancesMessage = document.getElementById('noInstancesMessage');
                if (noInstancesMessage) {
                    noInstancesMessage.style.display = 'none';
                }
            } else {
                // Mostrar mensagem se não houver instâncias
                instancesList.innerHTML = '';
                const noInstancesMessage = document.getElementById('noInstancesMessage');
                if (noInstancesMessage) {
                    noInstancesMessage.style.display = 'block';
                }
            }
        }
        
        // Atualizar lista mobile
        const mobileInstanceList = document.getElementById('mobileInstanceList');
        if (mobileInstanceList) {
            if (instancesState.instances.length > 0) {
                // Criar elementos para cada instância
                let html = '';
                instancesState.instances.forEach(instance => {
                    html += `
                        <a href="#" class="list-group-item list-group-item-action d-flex justify-content-between align-items-center mobile-instance-item" data-id="${instance.id}">
                            <div>
                                <h6 class="mb-1">${instance.name || `Instância ${instance.id}`}</h6>
                                <small class="text-muted">${instance.phone || 'Não conectada'}</small>
                            </div>
                            <div class="d-flex align-items-center">
                                <div class="status-dot ${instance.connected ? 'connected' : 'disconnected'} me-2"></div>
                                <small>${instance.connected ? (instance.loggedIn ? 'Conectada' : 'Aguardando QR') : 'Desconectada'}</small>
                            </div>
                        </a>
                    `;
                });
                mobileInstanceList.innerHTML = html;
                
                // Adicionar eventos de clique
                document.querySelectorAll('.mobile-instance-item').forEach(item => {
                    item.addEventListener('click', function(e) {
                        e.preventDefault();
                        const id = this.getAttribute('data-id');
                        selectInstance(id);
                        
                        // Fechar modal
                        const manageInstancesModal = document.getElementById('manageInstancesModal');
                        if (manageInstancesModal) {
                            const bsModal = bootstrap.Modal.getInstance(manageInstancesModal);
                            if (bsModal) bsModal.hide();
                        }
                    });
                });
                
                // Ocultar mensagem de "sem instâncias"
                const mobileNoInstancesMessage = document.getElementById('mobileNoInstancesMessage');
                if (mobileNoInstancesMessage) {
                    mobileNoInstancesMessage.style.display = 'none';
                }
            } else {
                // Mostrar mensagem se não houver instâncias
                mobileInstanceList.innerHTML = '';
                const mobileNoInstancesMessage = document.getElementById('mobileNoInstancesMessage');
                if (mobileNoInstancesMessage) {
                    mobileNoInstancesMessage.style.display = 'block';
                }
            }
        }
        
        // Atualizar tabela de instâncias para admin
        updateAdminInstancesList(instancesState.instances);
        
        // Atualizar contador de estatísticas
        const totalInstancesCount = document.getElementById('totalInstancesCount');
        if (totalInstancesCount) {
            totalInstancesCount.textContent = instancesState.instances.length;
        }
        
        const connectedInstancesCount = document.getElementById('connectedInstancesCount');
        if (connectedInstancesCount) {
            const connected = instancesState.instances.filter(i => i.connected).length;
            connectedInstancesCount.textContent = connected;
        }
        
        const disconnectedInstancesCount = document.getElementById('disconnectedInstancesCount');
        if (disconnectedInstancesCount) {
            const disconnected = instancesState.instances.filter(i => !i.connected).length;
            disconnectedInstancesCount.textContent = disconnected;
        }
    } else {
        // Para usuário normal, só precisa atualizar a interface da instância única
        const userInstance = instancesState.instances.find(i => i.id == window.authState.userId);
        if (userInstance) {
            console.log('[INSTANCES] Instância do usuário encontrada:', userInstance);
            updateInstanceDetailUI(userInstance);
        }
    }
}

/**
 * Preparar o container de dashboard se ainda não existir
 */
function prepareInstanceDashboard() {
    const container = document.getElementById('instanceDashboard');
    if (!container) {
        console.error('[INSTANCES] Container do dashboard de instância não encontrado');
        return;
    }
    
    // Se o usuário não for admin, expandir o dashboard para usar todo o espaço disponível
    if (!window.authState?.isAdmin) {
        const mainContent = document.querySelector('.col-md-9.col-lg-10');
        if (mainContent) {
            mainContent.className = 'col-12';
        }
        
        // Ocultar a coluna lateral se existir
        const sidebar = document.querySelector('.col-md-3.col-lg-2');
        if (sidebar) {
            sidebar.style.display = 'none';
        }
    }
}

/**
 * Atualizar detalhes da instância na UI
 * @param {object} instance - Dados da instância
 */
function updateInstanceDetailUI(instance) {
    if (!instance) return;
    
    // Atualizar nome da instância
    const instanceDetailName = document.getElementById('instanceDetailName');
    if (instanceDetailName) {
        instanceDetailName.textContent = instance.name || `Instância ${instance.id}`;
    }
    
    // Atualizar indicador de status
    const statusDetail = document.getElementById('statusDetail');
    if (statusDetail) {
        statusDetail.className = `status-indicator ${instance.connected ? 'status-connected' : 'status-disconnected'} me-2`;
    }
    
    // Atualizar texto de status
    const statusDetailText = document.getElementById('statusDetailText');
    if (statusDetailText) {
        statusDetailText.textContent = instance.connected ? 
            (instance.loggedIn ? 'Conectada' : 'Aguardando QR') : 'Desconectada';
    }
    
    // Atualizar status de conexão
    const connectedStatus = document.getElementById('connectedStatus');
    if (connectedStatus) {
        connectedStatus.className = `badge ${instance.connected ? 'bg-success' : 'bg-danger'}`;
        connectedStatus.textContent = instance.connected ? 'Sim' : 'Não';
    }
    
    // Atualizar status de login
    const loggedInStatus = document.getElementById('loggedInStatus');
    if (loggedInStatus) {
        loggedInStatus.className = `badge ${instance.loggedIn ? 'bg-success' : 'bg-danger'}`;
        loggedInStatus.textContent = instance.loggedIn ? 'Sim' : 'Não';
    }
    
    // Atualizar número de telefone
    const phoneStatus = document.getElementById('phoneStatus');
    if (phoneStatus) {
        phoneStatus.textContent = instance.phone || 'N/A';
    }
    
    // Atualizar status do webhook
    const webhookStatus = document.getElementById('webhookStatus');
    if (webhookStatus) {
        webhookStatus.className = `badge ${instance.webhook ? 'bg-success' : 'bg-warning'}`;
        webhookStatus.textContent = instance.webhook ? 'Configurado' : 'Não Configurado';
    }
    
    // Atualizar campo de webhook
    const webhookUrl = document.getElementById('webhookUrl');
    if (webhookUrl) {
        webhookUrl.value = instance.webhook || '';
    }
    
    // Atualizar checkboxes de eventos
    if (instance.events && Array.isArray(instance.events)) {
        // Limpar todos os checkboxes primeiro
        document.querySelectorAll('.webhook-event').forEach(checkbox => {
            checkbox.checked = false;
        });
        
        // Marcar os eventos ativos
        instance.events.forEach(event => {
            const checkbox = document.getElementById(`event${event}`);
            if (checkbox) {
                checkbox.checked = true;
            }
        });
        
        // Verificar se todos estão marcados para o checkbox "todos"
        const allEvents = ['Message', 'ReadReceipt', 'Presence', 'ChatPresence', 'HistorySync'];
        const allChecked = allEvents.every(event => instance.events.includes(event));
        
        const eventAll = document.getElementById('eventAll');
        if (eventAll) {
            eventAll.checked = allChecked;
        }
    }
    
    // Atualizar seção de QR Code com base no status
    updateQrCodeSection(instance.connected, instance.loggedIn);
    
    // Carregar QR code se necessário (conectado mas não logado)
    if (instance.connected && !instance.loggedIn) {
        getQrCode();
    }
}

/**
 * Atualizar visibilidade e estado da seção do QR Code
 * @param {boolean} connected - Se a instância está conectada
 * @param {boolean} loggedIn - Se a instância está logada
 */
function updateQrCodeSection(connected, loggedIn) {
    const qrCodeSection = document.getElementById('qrCodeSection');
    if (!qrCodeSection) return;
    
    const qrCodeLoading = document.getElementById('qrCodeLoading');
    const qrCodeNotAvailable = document.getElementById('qrCodeNotAvailable');
    const getQrBtn = document.getElementById('getQrBtn');
    
    if (connected && !loggedIn) {
        // Mostrar seção de QR code quando conectado mas não logado
        qrCodeSection.style.display = '';
        
        // Mostrar loading e ocultar mensagem de indisponibilidade
        if (qrCodeLoading) qrCodeLoading.style.display = '';
        if (qrCodeNotAvailable) qrCodeNotAvailable.classList.add('d-none');
        
        // Mostrar botão de QR
        if (getQrBtn) getQrBtn.style.display = '';
    } else if (!connected) {
        // Quando desconectado, mostrar seção com mensagem de indisponibilidade
        qrCodeSection.style.display = '';
        
        // Ocultar loading e mostrar mensagem de indisponibilidade
        if (qrCodeLoading) qrCodeLoading.style.display = 'none';
        if (qrCodeNotAvailable) qrCodeNotAvailable.classList.remove('d-none');
        
        // Ocultar botão de QR
        if (getQrBtn) getQrBtn.style.display = 'none';
    } else if (connected && loggedIn) {
        // Quando já logado, ocultar a seção de QR code
        qrCodeSection.style.display = 'none';
    }
}

/**
 * Registrar eventos para o dashboard de instância
 */
function registerInstanceEvents() {
    // Evento para verificar status
    const checkStatusBtn = document.getElementById('checkStatusBtn');
    if (checkStatusBtn) {
        checkStatusBtn.addEventListener('click', () => {
            if (instancesState.currentInstance) {
                checkStatus(instancesState.currentInstance);
            }
        });
    }
    
    // Evento para conectar
    const connectBtn = document.getElementById('connectBtn');
    if (connectBtn) {
        connectBtn.addEventListener('click', () => {
            if (instancesState.currentInstance) {
                connect();
            }
        });
    }
    
    // Evento para desconectar
    const disconnectBtn = document.getElementById('disconnectBtn');
    if (disconnectBtn) {
        disconnectBtn.addEventListener('click', () => {
            if (instancesState.currentInstance) {
                disconnect();
            }
        });
    }
    
    // Evento para logout
    const logoutBtn = document.getElementById('logoutBtn');
    if (logoutBtn) {
        logoutBtn.addEventListener('click', () => {
            if (instancesState.currentInstance) {
                if (confirm('Tem certeza que deseja fazer logout desta instância do WhatsApp? Você precisará escanear o QR code novamente.')) {
                    logout();
                }
            }
        });
    }
    
    // Evento para parear por telefone
    const pairPhoneBtn = document.getElementById('pairPhoneBtn');
    if (pairPhoneBtn) {
        pairPhoneBtn.addEventListener('click', () => {
            if (typeof window.pairPhone === 'function') {
                window.pairPhone();
            }
        });
    }
    
    // Evento para obter QR code
    const getQrBtn = document.getElementById('getQrBtn');
    if (getQrBtn) {
        getQrBtn.addEventListener('click', getQrCode);
    }
    
    // Evento para salvar webhook
    const saveWebhookBtn = document.getElementById('saveWebhookBtn');
    if (saveWebhookBtn) {
        saveWebhookBtn.addEventListener('click', saveWebhook);
    }
    
    // Evento para "todos os eventos"
    const eventAll = document.getElementById('eventAll');
    if (eventAll) {
        eventAll.addEventListener('change', function() {
            const checked = this.checked;
            document.querySelectorAll('.webhook-event').forEach(checkbox => {
                checkbox.checked = checked;
            });
        });
    }
    
    // Eventos para atualizar o checkbox "todos" quando os outros mudam
    document.querySelectorAll('.webhook-event').forEach(checkbox => {
        checkbox.addEventListener('change', updateAllEventsCheckbox);
    });
    
    // Evento para enviar mensagem de teste
    const sendTestMsgBtn = document.getElementById('sendTestMsgBtn');
    if (sendTestMsgBtn) {
        sendTestMsgBtn.addEventListener('click', sendTestMessage);
    }
    
    // Evento para botão de prefixo de telefone
    const prefixBtn = document.getElementById('prefixBtn');
    if (prefixBtn) {
        prefixBtn.addEventListener('click', () => {
            const testNumber = document.getElementById('testNumber');
            if (testNumber) {
                testNumber.value = '55' + (testNumber.value.startsWith('55') ? testNumber.value.substring(2) : testNumber.value);
            }
        });
    }
}

/**
 * Atualizar o checkbox "todos os eventos" com base nos outros checkboxes
 */
function updateAllEventsCheckbox() {
    const allEvents = ['Message', 'ReadReceipt', 'Presence', 'ChatPresence', 'HistorySync'];
    const allChecked = allEvents.every(event => {
        const checkbox = document.getElementById(`event${event}`);
        return checkbox && checkbox.checked;
    });
    
    const eventAll = document.getElementById('eventAll');
    if (eventAll) {
        eventAll.checked = allChecked;
    }
}

/**
 * Salvar configuração de webhook
 */
async function saveWebhook() {
    if (!instancesState.currentInstance) {
        showToast('Nenhuma instância selecionada', 'warning');
        return;
    }
    
    // Obter URL do webhook
    const webhookUrl = document.getElementById('webhookUrl');
    if (!webhookUrl) return;
    
    // Obter eventos selecionados
    const selectedEvents = [];
    document.querySelectorAll('.webhook-event:checked').forEach(checkbox => {
        selectedEvents.push(checkbox.value);
    });
    
    // Preparar dados para a requisição
    const data = {
        webhook: webhookUrl.value.trim(),
        events: selectedEvents
    };
    
    // Verificar se a URL é válida quando fornecida
    if (data.webhook && !isValidUrl(data.webhook)) {
        showToast('URL do webhook inválida', 'warning');
        return;
    }
    
    showLoading('Salvando configurações de webhook...');
    
    try {
        // Chamar API para atualizar webhook
        const response = await makeApiCall(`/instances/${instancesState.currentInstance}/webhook`, 'POST', data);
        
        hideLoading();
        
        if (response.success) {
            showToast('Webhook configurado com sucesso', 'success');
            addLog('Configuração de webhook atualizada', 'success');
            
            // Atualizar estado local
            const instance = instancesState.instances.find(i => i.id == instancesState.currentInstance);
            if (instance) {
                instance.webhook = data.webhook;
                instance.events = data.events;
                
                // Atualizar UI
                updateInstanceDetailUI(instance);
            }
            
            // Atualizar status do webhook na UI
            const webhookStatus = document.getElementById('webhookStatus');
            if (webhookStatus) {
                webhookStatus.className = `badge ${data.webhook ? 'bg-success' : 'bg-warning'}`;
                webhookStatus.textContent = data.webhook ? 'Configurado' : 'Não Configurado';
            }
        } else {
            showToast(`Erro ao configurar webhook: ${response.error || 'Erro desconhecido'}`, 'danger');
            addLog(`Erro ao configurar webhook: ${response.error || 'Erro desconhecido'}`, 'error');
        }
    } catch (error) {
        hideLoading();
        showToast(`Erro ao configurar webhook: ${error.message}`, 'danger');
        addLog(`Erro ao configurar webhook: ${error.message}`, 'error');
    }
}

/**
 * Verificar se uma string é uma URL válida
 * @param {string} url - URL para validar
 * @returns {boolean} Se é uma URL válida
 */
function isValidUrl(url) {
    try {
        new URL(url);
        return true;
    } catch (e) {
        return false;
    }
}

/**
 * Enviar mensagem de teste
 */
async function sendTestMessage() {
    if (!instancesState.currentInstance) {
        showToast('Nenhuma instância selecionada', 'warning');
        return;
    }
    
    // Verificar se está conectado e logado
    const instance = instancesState.instances.find(i => i.id == instancesState.currentInstance);
    if (!instance || !instance.connected || !instance.loggedIn) {
        showToast('A instância precisa estar conectada e logada para enviar mensagens', 'warning');
        return;
    }
    
    // Obter dados do formulário
    const number = document.getElementById('testNumber')?.value.trim();
    const messageType = document.getElementById('testMessageType')?.value;
    const message = document.getElementById('testMessage')?.value.trim();
    
    if (!number) {
        showToast('Informe um número de telefone', 'warning');
        return;
    }
    
    if (!message && messageType === 'text') {
        showToast('Informe uma mensagem', 'warning');
        return;
    }
    
    // Limpar e formatar o número
    const formattedNumber = number.replace(/\D/g, '');
    
    if (formattedNumber.length < 10) {
        showToast('Número de telefone inválido. Formato: DDI+DDD+Número', 'warning');
        return;
    }
    
    showLoading('Enviando mensagem de teste...');
    
    try {
        // Preparar dados de acordo com o tipo de mensagem
        let endpoint = `/instances/${instancesState.currentInstance}/chat/send`;
        let data = {
            number: formattedNumber
        };
        
        switch (messageType) {
            case 'text':
                endpoint += '/text';
                data.text = message;
                break;
            case 'image':
                endpoint += '/image';
                data.url = message;
                data.caption = 'Mensagem de teste';
                break;
            case 'document':
                endpoint += '/document';
                data.url = message;
                data.filename = 'documento_teste.pdf';
                break;
            case 'location':
                endpoint += '/location';
                const [latitude, longitude] = message.split(',');
                data.latitude = parseFloat(latitude || '-23.5505');
                data.longitude = parseFloat(longitude || '-46.6333');
                data.name = 'Localização de teste';
                break;
        }
        
        // Chamar API para enviar mensagem
        const response = await makeApiCall(endpoint, 'POST', data);
        
        hideLoading();
        
        if (response.success) {
            showToast('Mensagem enviada com sucesso', 'success');
            addLog('Mensagem de teste enviada com sucesso', 'success');
            
            // Limpar mensagem após o envio
            const messageInput = document.getElementById('testMessage');
            if (messageInput) {
                messageInput.value = '';
            }
        } else {
            showToast(`Erro ao enviar mensagem: ${response.error || 'Erro desconhecido'}`, 'danger');
            addLog(`Erro ao enviar mensagem: ${response.error || 'Erro desconhecido'}`, 'error');
        }
    } catch (error) {
        hideLoading();
        showToast(`Erro ao enviar mensagem: ${error.message}`, 'danger');
        addLog(`Erro ao enviar mensagem: ${error.message}`, 'error');
    }
}

/**
 * Extrai o número de telefone do formato JID
 * @param {string} jid - O formato JID completo
 * @returns {string} O número de telefone extraído
 */
function extractPhoneFromJid(jid) {
    if (!jid) return '';
    
    // Verificar se há o caractere ":" que indica recursos específicos no JID
    const colonIndex = jid.indexOf(':');
    if (colonIndex > 0) {
        jid = jid.substring(0, colonIndex);
    }
    
    // Remove a parte @s.whatsapp.net
    const atIndex = jid.indexOf('@');
    if (atIndex > 0) {
        return jid.substring(0, atIndex);
    }
    
    return jid;
}

/**
 * Exibir o dashboard da instância selecionada
 */
function showInstanceDashboard() {
    // Buscar a instância selecionada
    const instance = instancesState.instances.find(i => i.id == instancesState.currentInstance);
    
    if (!instance) {
        console.warn(`[INSTANCES] Instância selecionada ${instancesState.currentInstance} não encontrada para mostrar dashboard`);
        return;
    }
    
    // Verificar se o container já existe
    const container = document.getElementById('instanceDashboard');
    
    if (!container) {
        console.error('[INSTANCES] Container do dashboard de instância não encontrado');
        return;
    }
    
    // Limpar conteúdo anterior
    container.innerHTML = '';
    
    // Criar conteúdo do dashboard
    const dashboardHTML = `
        <div class="col-12 mb-4">
            <div class="card">
                <div class="card-header">
                    <div class="d-flex justify-content-between align-items-center">
                        <div>
                            <i class="bi bi-phone me-2"></i>
                            <span id="instanceDetailName">${instance.name || `Instância ${instance.id}`}</span>
                        </div>
                        <div class="d-flex align-items-center">
                            <div class="status-indicator ${instance.connected ? 'status-connected' : 'status-disconnected'} me-2" id="statusDetail"></div>
                            <span id="statusDetailText">${instance.connected ? (instance.loggedIn ? 'Conectada' : 'Aguardando QR') : 'Desconectada'}</span>
                            <button class="btn btn-sm btn-outline-secondary ms-2" id="checkStatusBtn">
                                <i class="bi bi-arrow-clockwise"></i>
                            </button>
                        </div>
                    </div>
                </div>
                <div class="card-body">
                    <div class="row">
                        <div class="col-lg-6 mb-4">
                            <div class="card connection-status-card">
                                <div class="card-body">
                                    <h5 class="card-title">Status da Conexão</h5>
                                    <div class="row mb-4">
                                        <div class="col-md-6 mb-3">
                                            <div class="d-flex justify-content-between align-items-center">
                                                <span>Conectado:</span>
                                                <span class="badge ${instance.connected ? 'bg-success' : 'bg-danger'}" id="connectedStatus">
                                                    ${instance.connected ? 'Sim' : 'Não'}
                                                </span>
                                            </div>
                                        </div>
                                        <div class="col-md-6 mb-3">
                                            <div class="d-flex justify-content-between align-items-center">
                                                <span>Logado:</span>
                                                <span class="badge ${instance.loggedIn ? 'bg-success' : 'bg-danger'}" id="loggedInStatus">
                                                    ${instance.loggedIn ? 'Sim' : 'Não'}
                                                </span>
                                            </div>
                                        </div>
                                        <div class="col-md-6 mb-3">
                                            <div class="d-flex justify-content-between align-items-center">
                                                <span>Número:</span>
                                                <span class="badge bg-primary" id="phoneStatus">
                                                    ${instance.phone || 'N/A'}
                                                </span>
                                            </div>
                                        </div>
                                        <div class="col-md-6 mb-3">
                                            <div class="d-flex justify-content-between align-items-center">
                                                <span>Webhook:</span>
                                                <span class="badge ${instance.webhook ? 'bg-success' : 'bg-warning'}" id="webhookStatus">
                                                    ${instance.webhook ? 'Configurado' : 'Não Configurado'}
                                                </span>
                                            </div>
                                        </div>
                                    </div>
                                    <div class="d-flex flex-wrap gap-2">
                                        <button class="btn btn-success action-btn" id="connectBtn" data-condition="disconnected">
                                            <i class="bi bi-power"></i>Conectar
                                        </button>
                                        <button class="btn btn-warning action-btn" id="disconnectBtn" data-condition="connected">
                                            <i class="bi bi-pause-circle"></i>Desconectar
                                        </button>
                                        <button class="btn btn-danger action-btn" id="logoutBtn" data-condition="logged-in">
                                            <i class="bi bi-box-arrow-right"></i>Logout
                                        </button>
                                        <button class="btn btn-info text-white action-btn" id="pairPhoneBtn" data-condition="disconnected">
                                            <i class="bi bi-phone"></i>Parear por Telefone
                                        </button>
                                    </div>
                                </div>
                            </div>
                        </div>
                        <div class="col-lg-6 mb-4">
                            <div class="card qr-code-card" id="qrCodeSection">
                                <div class="card-body text-center">
                                    <h5 class="card-title">QR Code</h5>
                                    <div id="qrCode" class="qr-container mt-3">
                                        <div class="text-center mb-3" id="qrCodeLoading">
                                            <div class="spinner-border text-primary" role="status"></div>
                                            <p class="mt-2">Aguardando QR Code...</p>
                                        </div>
                                        <div class="text-center d-none" id="qrCodeNotAvailable">
                                            <i class="bi bi-qr-code text-muted" style="font-size: 5rem;"></i>
                                            <p class="mt-2">QR Code não disponível.</p>
                                            <small class="text-muted">Conecte para gerar um novo QR Code.</small>
                                        </div>
                                    </div>
                                    <div class="mt-3">
                                        <button class="btn btn-primary action-btn" id="getQrBtn" data-condition="waiting-qr">
                                            <i class="bi bi-qr-code"></i>Obter QR Code
                                        </button>
                                    </div>
                                </div>
                            </div>
                        </div>
                    </div>
                    <div class="row">
                        <div class="col-lg-6 mb-4">
                            <div class="card webhook-card">
                                <div class="card-body">
                                    <h5 class="card-title">Configuração de Webhook</h5>
                                    <div class="mb-3">
                                        <label for="webhookUrl" class="form-label">URL do Webhook</label>
                                        <input type="url" class="form-control" id="webhookUrl" placeholder="https://seu-servidor.com/webhook" value="${instance.webhook || ''}">
                                    </div>
                                    <div class="mb-3">
                                        <label class="form-label">Eventos</label>
                                        <div class="row">
                                            <div class="col-md-6">
                                                <div class="form-check mb-2">
                                                    <input class="form-check-input webhook-event" type="checkbox" value="Message" id="eventMessage" ${instance.events && instance.events.includes('Message') ? 'checked' : ''}>
                                                    <label class="form-check-label" for="eventMessage">
                                                        Mensagens
                                                    </label>
                                                </div>
                                                <div class="form-check mb-2">
                                                    <input class="form-check-input webhook-event" type="checkbox" value="ReadReceipt" id="eventReadReceipt" ${instance.events && instance.events.includes('ReadReceipt') ? 'checked' : ''}>
                                                    <label class="form-check-label" for="eventReadReceipt">
                                                        Confirmações de Leitura
                                                    </label>
                                                </div>
                                                <div class="form-check mb-2">
                                                    <input class="form-check-input webhook-event" type="checkbox" value="HistorySync" id="eventHistorySync" ${instance.events && instance.events.includes('HistorySync') ? 'checked' : ''}>
                                                    <label class="form-check-label" for="eventHistorySync">
                                                        Sincronização de Histórico
                                                    </label>
                                                </div>
                                            </div>
                                            <div class="col-md-6">
                                                <div class="form-check mb-2">
                                                    <input class="form-check-input webhook-event" type="checkbox" value="Presence" id="eventPresence" ${instance.events && instance.events.includes('Presence') ? 'checked' : ''}>
                                                    <label class="form-check-label" for="eventPresence">
                                                        Presença
                                                    </label>
                                                </div>
                                                <div class="form-check mb-2">
                                                    <input class="form-check-input webhook-event" type="checkbox" value="ChatPresence" id="eventChatPresence" ${instance.events && instance.events.includes('ChatPresence') ? 'checked' : ''}>
                                                    <label class="form-check-label" for="eventChatPresence">
                                                        Presença em Chat
                                                    </label>
                                                </div>
                                                <div class="form-check mb-2">
                                                    <input class="form-check-input" type="checkbox" value="All" id="eventAll">
                                                    <label class="form-check-label" for="eventAll">
                                                        Todos os Eventos
                                                    </label>
                                                </div>
                                            </div>
                                        </div>
                                    </div>
                                    <button class="btn btn-primary" id="saveWebhookBtn">
                                        <i class="bi bi-save"></i>Salvar Webhook
                                    </button>
                                </div>
                            </div>
                        </div>
                        <div class="col-lg-6 mb-4">
                            <div class="card send-message-card">
                                <div class="card-body">
                                    <h5 class="card-title">Enviar Mensagem de Teste</h5>
                                    <div class="mb-3">
                                        <label for="testNumber" class="form-label">Número WhatsApp</label>
                                        <div class="input-group">
                                            <input type="text" class="form-control" id="testNumber" placeholder="5521999999999">
                                            <button class="btn btn-outline-secondary" type="button" id="prefixBtn">
                                                <i class="bi bi-telephone"></i>
                                            </button>
                                        </div>
                                        <div class="form-text">Formato: DDI + DDD + Número (ex: 5521999999999)</div>
                                    </div>
                                    <div class="mb-3">
                                        <label for="testMessageType" class="form-label">Tipo de Mensagem</label>
                                        <select class="form-select" id="testMessageType">
                                            <option value="text" selected>Texto</option>
                                            <option value="image">Imagem</option>
                                            <option value="document">Documento</option>
                                            <option value="location">Localização</option>
                                        </select>
                                    </div>
                                    <div class="mb-3">
                                        <label for="testMessage" class="form-label">Mensagem</label>
                                        <textarea class="form-control" id="testMessage" rows="2" placeholder="Digite sua mensagem de teste"></textarea>
                                    </div>
                                    <button class="btn btn-primary action-btn" id="sendTestMsgBtn" data-condition="logged-in">
                                        <i class="bi bi-send"></i>Enviar Mensagem de Teste
                                    </button>
                                </div>
                            </div>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    `;
    
    // Adicionar conteúdo ao container
    container.innerHTML = dashboardHTML;
    
    // Mostrar container
    container.style.display = 'flex';
    
    // Registrar eventos
    registerInstanceEvents();
    
    // Mostrar ou ocultar seção do QR Code com base no status de conexão
    updateQrCodeSection(instance.connected, instance.loggedIn);
}

/**
 * Ocultar o dashboard da instância
 */
function hideInstanceDashboard() {
    const noInstanceSelected = document.getElementById('noInstanceSelected');
    const instanceDashboard = document.getElementById('instanceDashboard');
    const currentInstanceName = document.getElementById('currentInstanceName');
    
    if (noInstanceSelected) {
        noInstanceSelected.style.display = 'block';
    }
    if (instanceDashboard) {
        instanceDashboard.style.display = 'none';
    }
    if (currentInstanceName) {
        currentInstanceName.textContent = 'Nenhuma instância selecionada';
    }
}

/**
 * Criar uma nova instância
 * @param {object} instanceData - Dados da instância a ser criada
 * @returns {Promise<object|null>} Dados da instância criada ou null
 */
async function createInstance(instanceData = {}) {
    if (!getAuthState().isAuthenticated) {
        showToast('Token de autenticação necessário', 'warning');
        return null;
    }
    
    // Verificar se o usuário é admin (pode criar instâncias)
    if (!authState?.isAdmin) {
        showToast('Apenas administradores podem criar instâncias', 'warning');
        addLog('Tentativa de criar instância sem permissão de administrador', 'error');
        return null;
    }
    
    if (!instanceData.name) {
        showToast('Nome da instância é obrigatório', 'warning');
        return null;
    }
    
    try {
        showLoading('Criando nova instância...');
        
        // Preparar payload
        const payload = {
            name: instanceData.name,
            token: instanceData.token || "" // Deixar o servidor gerar o token automaticamente se não for fornecido
        };
        
        // Adicionar webhook se fornecido
        if (instanceData.webhook) {
            payload.webhook = instanceData.webhook;
        }
        
        // Adicionar eventos se fornecidos
        if (instanceData.events && instanceData.events.length > 0) {
            // Enviar como string separada por vírgulas para compatibilidade com o backend
            payload.events = instanceData.events.join(',');
        }
        
        // Adicionar proxy se fornecido e habilitado
        if (instanceData.proxyUrl && instanceData.proxyEnabled) {
            payload.proxy_url = instanceData.proxyUrl;
            payload.proxy_enabled = true;
        }
        
        const response = await makeApiCall('/instances/create', 'POST', payload);
        
        hideLoading();
        
        if (response.success && response.data && response.data.id) {
            addLog(`Instância "${instanceData.name}" criada com sucesso. ID: ${response.data.id}`, 'success');
            showToast(`Instância "${instanceData.name}" criada com sucesso`, 'success');
            
            // Recarregar lista de instâncias e selecionar a nova
            await loadInstances();
            selectInstance(response.data.id);
            
            return response.data;
        } else {
            const errorMsg = response.error || 'Erro desconhecido';
            console.error('Erro na criação da instância:', response);
            addLog(`Falha ao criar instância: ${errorMsg}`, 'error');
            showToast(`Falha ao criar instância: ${errorMsg}`, 'danger');
            return null;
        }
    } catch (error) {
        hideLoading();
        console.error('Erro ao criar instância:', error);
        addLog(`Erro ao criar instância: ${error.message || 'Erro desconhecido'}`, 'error');
        showToast('Erro ao criar instância', 'danger');
        return null;
    }
}

/**
 * Gerar um token aleatório para instância
 * @param {number} length - Tamanho do token
 * @returns {string} Token gerado
 */
function generateInstanceToken(length = 12) {
    const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789';
    let token = '';
    
    for (let i = 0; i < length; i++) {
        token += chars.charAt(Math.floor(Math.random() * chars.length));
    }
    
    return token;
}

/**
 * Excluir uma instância
 * @param {string} instanceId - ID da instância a ser excluída
 * @returns {Promise<boolean>} Sucesso da operação
 */
async function deleteInstance(instanceId) {
    if (!instanceId || instanceId === '') {
        addLog('ID da instância não fornecido para exclusão', 'error');
        showToast('ID da instância não fornecido', 'danger');
        return false;
    }
    
    try {
        // Confirmar antes de excluir
        if (!confirm(`Tem certeza que deseja excluir completamente a instância ${instanceId}? Esta ação não pode ser desfeita e removerá todos os dados associados.`)) {
            return false;
        }
        
        showLoading(`Excluindo instância ${instanceId}...`);
        addLog(`Iniciando exclusão completa da instância ${instanceId}...`);
        
        // Primeiro, desconectar a instância se estiver conectada
        const statusCheck = await makeApiCall(`/instances/${instanceId}/status`, 'GET');
        if (statusCheck.success && 
            (statusCheck.data?.Connected === true || statusCheck.data?.connected === true)) {
            addLog('Desconectando instância antes da exclusão...', 'info');
            
            // Desconectar
            await makeApiCall(`/instances/${instanceId}/disconnect`, 'POST');
            
            // Pequena pausa para garantir que a desconexão seja processada
            await new Promise(resolve => setTimeout(resolve, 1000));
            
            // Fazer logout para limpar sessão
            await makeApiCall(`/instances/${instanceId}/logout`, 'POST');
            
            // Outra pequena pausa
            await new Promise(resolve => setTimeout(resolve, 1000));
        }
        
        // Agora, excluir a instância e todos os dados associados
        // Usar o endpoint específico que limpa também os arquivos da sessão
        const deleteUrl = authState.isAdmin 
            ? `/admin/user/${instanceId}/delete-complete` 
            : `/instances/${instanceId}/delete-complete`;
            
        const response = await makeApiCall(deleteUrl, 'DELETE');
        
        hideLoading();
        
        if (response.success) {
            addLog(`Instância ${instanceId} excluída com sucesso`, 'success');
            showToast('Instância excluída com sucesso', 'success');
            
            // Atualizar a lista de instâncias
            await loadInstances();
            
            // Se a instância excluída for a atual, limpar a seleção
            if (instancesState.currentInstance === instanceId) {
                    instancesState.currentInstance = null;
                localStorage.removeItem('currentInstance');
                window.currentInstance = null;
                
                // Ocultar dashboard de instância
                    hideInstanceDashboard();
                
                // Mostrar mensagem de nenhuma instância selecionada
                showNoInstancesMessage();
            }
            
            return true;
        } else {
            const errorMsg = response.error || 'Erro desconhecido';
            addLog(`Falha ao excluir instância ${instanceId}: ${errorMsg}`, 'error');
            showToast(`Falha ao excluir instância: ${errorMsg}`, 'danger');
            return false;
        }
    } catch (error) {
        hideLoading();
        addLog(`Erro ao excluir instância ${instanceId}: ${error.message}`, 'error');
        showToast('Erro ao excluir instância', 'danger');
        return false;
    }
}

/**
 * Atualizar o nome de uma instância
 * @param {string} instanceId - ID da instância
 * @param {string} name - Novo nome
 * @returns {Promise<boolean>} Sucesso da operação
 */
async function updateInstanceName(instanceId, name) {
    if (!instanceId || !name) return false;
    
    // Verificar se o usuário é admin (pode atualizar instâncias)
    // Exceção: permitir que usuários normais atualizem o nome de suas próprias instâncias
    const isOwnInstance = instancesState.instances.some(instance => 
        instance.id === instanceId && instance.owner === authState.userToken
    );
    
    if (!authState?.isAdmin && !isOwnInstance) {
        showToast('Você não tem permissão para editar esta instância', 'warning');
        addLog('Tentativa de editar instância sem permissão', 'error');
        return false;
    }
    
    try {
        const response = await makeApiCall(`/instances/${instanceId}/update`, 'POST', {
            name: name
        });
        
        if (response.success) {
            addLog(`Nome da instância atualizado para "${name}"`, 'success');
            showToast(`Nome atualizado para "${name}"`, 'success');
            
            // Atualizar na lista
            const instance = instancesState.instances.find(i => i.id === instanceId);
            if (instance) {
                instance.name = name;
                renderInstanceList();
                renderMobileInstanceList();
            }
            
            // Atualizar nome exibido
            if (instancesState.currentInstance === instanceId) {
                const currentInstanceName = document.getElementById('currentInstanceName');
                if (currentInstanceName) {
                    currentInstanceName.textContent = name;
                }
            }
            
            return true;
        } else {
            addLog(`Falha ao atualizar nome: ${response.error || 'Erro desconhecido'}`, 'error');
            showToast(`Falha ao atualizar nome: ${response.error || 'Erro desconhecido'}`, 'danger');
            return false;
        }
    } catch (error) {
        addLog('Erro ao atualizar nome da instância', 'error');
        showToast('Erro ao atualizar nome da instância', 'danger');
        return false;
    }
}

/**
 * Verificar status da instância
 * @param {string} instanceId - ID da instância
 * @returns {Promise<object>} Status da instância
 */
async function checkStatus(instanceId) {
    if (!instanceId) return null;

    // Adicionar bloqueio de múltiplas chamadas simultâneas
    const checkId = `check_${instanceId}_${Date.now()}`;
    if (window.lastStatusCheck && window.lastStatusCheck.id === instanceId && 
        Date.now() - window.lastStatusCheck.time < 2000) {
        console.log(`[INSTANCES] Verificação de status recente para instância ${instanceId}, ignorando chamada redundante`);
        return window.lastStatusCheck.result;
    }
    
    window.lastStatusCheck = {
        id: instanceId,
        time: Date.now(),
        result: null
    };
    
    try {
        showLoading('Verificando status...');
        addLog(`Verificando status da instância ${instanceId}...`);
        
        const response = await makeApiCall(`/instances/${instanceId}/status`, 'GET');
        
        hideLoading();
        
        if (response.success) {
            // Processar resposta
            let isConnected = false;
            let isLoggedIn = false;
            let phone = '';
            
            // Processar formato da resposta
            if (response.data.hasOwnProperty('Connected')) {
                isConnected = response.data.Connected === true;
                isLoggedIn = response.data.LoggedIn === true;
                phone = response.data.phone || '';
            } else if (response.data.hasOwnProperty('connected')) {
                isConnected = response.data.connected === true;
                isLoggedIn = response.data.loggedIn === true;
                phone = response.data.phone || '';
            }
            
            // Se é uma resposta de fallback (quando a API falhou), marcar como erro
            if (response.data.isOfflineFallback) {
                addLog(`Não foi possível obter status atualizado para instância ${instanceId}. Assumindo desconectado.`, 'warning');
                isConnected = false;
                isLoggedIn = false;
            } else {
                addLog(`Status atualizado para instância ${instanceId}`, 'info');
            }
            
            // Atualizar UI apenas se for a instância atual
            if (instanceId === instancesState.currentInstance) {
                updateStatusUI(isConnected, isLoggedIn);
                
                // Se estiver aguardando QR e ainda não estiver conectado,
                // tentar obter QR code novamente
                const qrCodeSection = document.getElementById('qrCodeSection');
                if (isConnected && !isLoggedIn && qrCodeSection && 
                    qrCodeSection.style.display !== 'none' && 
                    !document.querySelector('#qrCode img')) {
                    getQrCode();
                }
                
                // Não iniciar polling aqui, pois isso será feito no selectInstance
                // Evitar chamadas redundantes que causam múltiplos intervalos
            }
            
            // Atualizar dados na lista de instâncias
            updateInstanceInList(instanceId, isConnected, isLoggedIn, phone);
            
            // Salvar estado no localStorage
            if (instanceId === instancesState.currentInstance) {
                localStorage.setItem('instanceStatus', JSON.stringify({
                    connected: isConnected,
                    loggedIn: isLoggedIn,
                    lastCheck: Date.now(),
                    phone: phone
                }));
            }
            
            // Se for a instância admin, salvar estado separadamente
            if (instanceId === '1' && authState.isAdmin) {
                localStorage.setItem('adminInstanceStatus', JSON.stringify({
                    connected: isConnected,
                    loggedIn: isLoggedIn,
                    lastCheck: Date.now(),
                    phone: phone
                }));
                
                // Resetar contador de tentativas se estiver conectado
                if (isConnected) {
                    instancesState.connectionAttempts = 0;
                }
            }
            
            // Salvar resultado para evitar verificações redundantes
            const result = {
                connected: isConnected,
                loggedIn: isLoggedIn,
                phone: phone,
                needsReconnect: needsReconnect
            };
            
            window.lastStatusCheck.result = result;
            return result;
        }
        
        // Em caso de erro, assumir desconectado
        updateStatusUI(false, false);
        
        addLog(`Falha ao verificar status: ${response.error || 'Erro desconhecido'}`, 'error');
        return {
            connected: false,
            loggedIn: false
        };
    } catch (error) {
        hideLoading();
        addLog(`Erro ao verificar status: ${error.message || 'Erro desconhecido'}`, 'error');
        
        // Em caso de erro, assumir desconectado
        updateStatusUI(false, false);
        
        return {
            connected: false,
            loggedIn: false
        };
    }
    
    window.lastStatusCheck.result = null;
    return null;
}

/**
 * Obter o código QR para a instância atual
 * @returns {Promise<string|null>} URL do QR code ou null
 */
async function getQrCode() {
    if (!instancesState.currentInstance) {
        addLog('Nenhuma instância selecionada', 'error');
        return null;
    }
    
    try {
        addLog('Obtendo QR code...');
        
        // Garantir que estamos usando o ID correto da instância
        const instanceId = instancesState.currentInstance;
        
        // Mostrar indicador de carregamento no elemento do QR code
        const qrCode = document.getElementById('qrCode');
        if (qrCode) {
            qrCode.innerHTML = `
                <div class="qr-loading text-center py-4">
                    <div class="loading-spinner mx-auto mb-3" style="width: 2.5rem; height: 2.5rem;"></div>
                    <p class="text-muted">Gerando QR code...</p>
                </div>
            `;
        }
        
        // Verificar se já está conectado - se não estiver, tentar conectar primeiro
        const statusResponse = await makeApiCall(`/instances/${instanceId}/status`, 'GET');
        let needsConnect = false;
        
        if (!statusResponse.success || 
            !(statusResponse.data?.Connected === true || statusResponse.data?.connected === true)) {
            needsConnect = true;
            addLog('Instância não está conectada. Iniciando conexão...', 'info');
        }
        
        // Se precisar conectar, fazemos isso antes de pedir o QR
        if (needsConnect) {
            const connectResponse = await makeApiCall(`/instances/${instanceId}/connect`, 'POST', {
                Subscribe: ["Message", "ReadReceipt", "Presence", "ChatPresence"],
                Immediate: true
            });
            
            if (!connectResponse.success) {
                if (qrCode) {
                    qrCode.innerHTML = `<div class="alert alert-danger">Erro ao conectar: ${connectResponse.error || 'Falha na conexão'}</div>`;
                }
                addLog(`Falha ao conectar para gerar QR: ${connectResponse.error || 'Erro desconhecido'}`, 'error');
                return null;
            }
            
            // Atualizar status na UI
            updateStatusUI(true, false);
            
            // Aguardar um momento para que a conexão seja estabelecida
            await new Promise(resolve => setTimeout(resolve, 1000));
        }
        
        // Agora solicitamos o QR code
        const response = await makeApiCall(`/instances/${instanceId}/qr`, 'GET');
        
        if (!qrCode) return null;
        
        if (response.success) {
            // Verificar os dois formatos possíveis de resposta
            let qrCodeUrl = response.data?.qrcode || response.data?.QRCode;
            
            if (qrCodeUrl) {
                // Garantir que a URL do QR code é válida
                if (qrCodeUrl.startsWith('data:image/')) {
                    qrCode.innerHTML = `<img src="${qrCodeUrl}" alt="QR Code" class="img-fluid qr-code-image">`;
                    addLog('QR code gerado com sucesso', 'success');
                    startQrCheck(instanceId);
                    return qrCodeUrl;
                } else {
                    // Talvez a URL esteja incompleta
                    qrCode.innerHTML = `<div class="alert alert-warning">QR code retornado em formato inválido</div>`;
                    addLog('Formato de QR code inválido', 'warning');
                    return null;
                }
            } else {
                // Se não tem QR code, verificar se já está logado
                if (response.data?.LoggedIn === true || response.data?.loggedIn === true) {
                    qrCode.innerHTML = '<div class="alert alert-success">Esta instância já está conectada e autenticada!</div>';
                } else {
                    qrCode.innerHTML = '<div class="alert alert-info">QR code não disponível. Aguarde alguns segundos e tente novamente, ou tente reconectar a instância.</div>';
                }
                addLog('QR code não disponível', 'info');
                return null;
            }
        } else {
            // Resposta com erro - verificar se temos mensagem específica
            let errorMsg = response.error || "Não foi possível obter o QR code";
            
            // Instruções mais úteis baseadas no tipo de erro
            let helpMsg = "";
            if (response.status === 404) {
                helpMsg = "Tente conectar a instância primeiro.";
            } else if (response.status === 500) {
                helpMsg = "Erro interno do servidor. Tente desconectar e conectar novamente.";
            }
            
            qrCode.innerHTML = `
                <div class="alert alert-danger">
                    <p>${errorMsg}</p>
                    ${helpMsg ? `<p><strong>Sugestão:</strong> ${helpMsg}</p>` : ''}
                    <button class="btn btn-primary mt-2" onclick="connect()">Tentar Conectar</button>
                </div>`;
            addLog(errorMsg, 'error');
            return null;
        }
    } catch (error) {
        const qrCode = document.getElementById('qrCode');
        if (qrCode) {
            qrCode.innerHTML = `
                <div class="alert alert-danger">
                    <p>Erro ao gerar QR code: ${error.message || 'Erro desconhecido'}</p>
                    <button class="btn btn-primary mt-2" onclick="getQrCode()">Tentar Novamente</button>
                </div>`;
        }
        addLog(`Erro ao obter QR code: ${error.message || 'Erro desconhecido'}`, 'error');
        return null;
    }
}

/**
 * Iniciar verificação periódica de status após exibir QR code
 * @param {string} instanceId - ID da instância a verificar (opcional, usa a atual se não fornecido)
 */
function startQrCheck(instanceId) {
    clearQrCheck();
    
    // Se não foi fornecido instanceId, usar a instância atual
    const idToCheck = instanceId || instancesState.currentInstance;
    
    if (!idToCheck) {
        addLog('Não foi possível iniciar verificação de QR: instância não identificada', 'error');
        return;
    }
    
    addLog(`Iniciando verificação periódica do status para instância ${idToCheck}...`, 'info');
    
    instancesState.qrCheckInterval = setInterval(async () => {
        // Verificar status diretamente pela API para esta instância específica
        const response = await makeApiCall(`/instances/${idToCheck}/status`, 'GET');
        
        let isLoggedIn = false;
        
        if (response.success) {
            // Verificar ambos os formatos possíveis de resposta
            if (response.data.hasOwnProperty('LoggedIn')) {
                isLoggedIn = response.data.LoggedIn === true;
            } else if (response.data.hasOwnProperty('loggedIn')) {
                isLoggedIn = response.data.loggedIn === true;
            }
            
            // Se logado, atualizar UI
            if (isLoggedIn) {
                clearQrCheck();
                
                const qrCode = document.getElementById('qrCode');
                if (qrCode) {
                    qrCode.innerHTML = '<div class="alert alert-success">Conectado com sucesso!</div>';
                }
                
                // Atualizar estado da instância na lista
                const instance = instancesState.instances.find(i => i.id === idToCheck);
                if (instance) {
                    instance.isConnected = true;
                    instance.isLoggedIn = true;
                    instance.connected = 1;
                    instance.loggedIn = true;
                    
                    // Atualizar interface
                    renderInstanceList();
                    renderMobileInstanceList();
                }
                
                // Atualizar instantaneamente a interface apenas se for a instância atual
                if (idToCheck === instancesState.currentInstance) {
                    updateStatusUI(true, true);
                }
                
                addLog(`Instância ${idToCheck} conectada e autenticada com sucesso`, 'success');
            }
        } else {
            addLog(`Erro ao verificar status de login para instância ${idToCheck}: ${response.error || 'Erro desconhecido'}`, 'warning');
        }
    }, 3000);
}

/**
 * Limpar o intervalo de verificação de QR code
 */
function clearQrCheck() {
    if (instancesState.qrCheckInterval) {
        clearInterval(instancesState.qrCheckInterval);
        instancesState.qrCheckInterval = null;
    }
}

/**
 * Sincronizar contatos da instância atual
 * @returns {Promise<number|null>} Número de contatos sincronizados ou null
 */
async function syncContacts() {
    if (!instancesState.currentInstance) {
        addLog('Nenhuma instância selecionada', 'error');
        showToast('Selecione uma instância primeiro', 'warning');
        return null;
    }
    
    try {
        showLoading('Sincronizando contatos...');
        addLog('Iniciando sincronização de contatos...');
        
        const response = await makeApiCall(`/user/contacts`, 'GET');
        
        hideLoading();
        
        if (response.success) {
            const contactCount = response.data && Array.isArray(response.data) ? response.data.length : 0;
            addLog(`Sincronização concluída. ${contactCount} contatos encontrados.`, 'success');
            showToast(`${contactCount} contatos sincronizados com sucesso`);
            return contactCount;
        } else {
            addLog(`Falha ao sincronizar contatos: ${response.error || 'Erro desconhecido'}`, 'error');
            showToast(`Falha ao sincronizar contatos: ${response.error || 'Erro desconhecido'}`, 'danger');
            return null;
        }
    } catch (error) {
        hideLoading();
        addLog('Erro ao sincronizar contatos', 'error');
        showToast('Erro ao sincronizar contatos', 'danger');
        return null;
    }
}

/**
 * Verificar completamente o sistema, incluindo status do servidor, banco de dados e conexões
 * @returns {Promise<object>} Resultado da verificação
 */
async function checkSystemStatus() {
    try {
        showLoading('Verificando status do sistema...');
        addLog('Iniciando verificação completa do sistema');
        
        // Verificar API e conectividade básica
        const apiResponse = await makeApiCall('/admin/status', 'GET');
        if (!apiResponse.success) {
        hideLoading();
            addLog('Falha ao se comunicar com a API', 'error');
            showToast('Falha ao verificar status do sistema', 'danger');
            return { 
                success: false, 
                error: 'Falha na comunicação com a API',
                details: {
                    api: false,
                    database: false,
                    whatsapp: false,
                    storage: false
                }
            };
        }
        
        // Verificar instâncias (representa check do banco de dados)
        const instancesResponse = await makeApiCall('/instances', 'GET');
        const databaseStatus = instancesResponse.success;
        
        // Verificar status do WhatsApp (usar uma instância aleatória, se disponível)
        let whatsappStatus = false;
        if (databaseStatus && instancesResponse.data && instancesResponse.data.length > 0) {
            // Pegar a primeira instância disponível
            const testInstance = instancesResponse.data[0];
            const whatsappCheck = await makeApiCall(`/instances/${testInstance.id}/status`, 'GET');
            // Se conseguir consultar o status (não importa se conectado ou não), o serviço está funcionando
            whatsappStatus = whatsappCheck.success;
        }
        
        // Resultados da verificação
        const results = {
            success: true,
            details: {
                timestamp: new Date().toISOString(),
                api: true,
                database: databaseStatus,
                whatsapp: whatsappStatus,
                storage: true, // Assumimos que o armazenamento está OK se a API está respondendo
                instances: databaseStatus ? instancesResponse.data.length : 0,
                connected_instances: 0
            }
        };
        
        // Contar instâncias conectadas
        if (databaseStatus && instancesResponse.data) {
            results.details.connected_instances = instancesResponse.data.filter(
                instance => instance.connected === 1 || instance.connected === true
            ).length;
        }
        
        hideLoading();
        addLog('Verificação do sistema concluída', 'success');
        
        // Exibir resultados na UI se necessário
        const systemStatusUI = document.getElementById('systemStatus');
        if (systemStatusUI) {
            systemStatusUI.innerHTML = `
                <div class="card mb-4">
                    <div class="card-header">
                        <i class="bi bi-activity me-2"></i> Status do Sistema
                    </div>
                    <div class="card-body">
                        <div class="row">
                            <div class="col-md-6">
                                <ul class="list-group">
                                    <li class="list-group-item d-flex justify-content-between align-items-center">
                                        API 
                                        <span class="badge ${results.details.api ? 'bg-success' : 'bg-danger'}">
                                            ${results.details.api ? 'Online' : 'Falha'}
                                        </span>
                                    </li>
                                    <li class="list-group-item d-flex justify-content-between align-items-center">
                                        Banco de Dados 
                                        <span class="badge ${results.details.database ? 'bg-success' : 'bg-danger'}">
                                            ${results.details.database ? 'Conectado' : 'Falha'}
                                        </span>
                                    </li>
                                    <li class="list-group-item d-flex justify-content-between align-items-center">
                                        WhatsApp 
                                        <span class="badge ${results.details.whatsapp ? 'bg-success' : 'bg-danger'}">
                                            ${results.details.whatsapp ? 'Funcionando' : 'Indisponível'}
                                        </span>
                                    </li>
                                </ul>
                            </div>
                            <div class="col-md-6">
                                <ul class="list-group">
                                    <li class="list-group-item d-flex justify-content-between align-items-center">
                                        Total de Instâncias
                                        <span class="badge bg-primary">${results.details.instances}</span>
                                    </li>
                                    <li class="list-group-item d-flex justify-content-between align-items-center">
                                        Instâncias Conectadas
                                        <span class="badge bg-success">${results.details.connected_instances}</span>
                                    </li>
                                    <li class="list-group-item d-flex justify-content-between align-items-center">
                                        Última Verificação
                                        <span class="badge bg-info">${new Date().toLocaleTimeString()}</span>
                                    </li>
                                </ul>
                            </div>
                        </div>
                    </div>
                </div>
            `;
        }
        
        return results;
    } catch (error) {
        hideLoading();
        addLog(`Erro durante verificação do sistema: ${error.message}`, 'error');
        showToast('Erro ao verificar sistema', 'danger');
        
        return {
            success: false,
            error: error.message,
            details: {
                api: false,
                database: false,
                whatsapp: false,
                storage: false
            }
        };
    }
}

/**
 * Par phone pairing - para conectar sem QR code
 * @param {string} phone - Número de telefone para pareamento
 * @returns {Promise<string|null>} Código de pareamento ou null
 */
async function pairPhone(phone) {
    if (!instancesState.currentInstance) {
        addLog('Nenhuma instância selecionada', 'error');
        showToast('Selecione uma instância primeiro', 'warning');
        return null;
    }
    
    if (!phone) {
        showToast('Digite um número de telefone válido', 'warning');
        return null;
    }
    
    try {
        showLoading('Gerando código de pareamento...');
        addLog(`Solicitando código de pareamento para ${phone}...`);
        
        // Usar o formato correto da API
        const response = await makeApiCall(`/instances/${instancesState.currentInstance}/pairphone`, 'POST', {
            Phone: phone // Primeira letra maiúscula como no backend
        });
        
        hideLoading();
        
        // Processar resposta no formato da API
        if (response.success) {
            const linkingCode = response.data?.LinkingCode || response.data?.linkingCode;
            
            if (linkingCode) {
                addLog('Código de pareamento gerado com sucesso', 'success');
                showToast('Código de pareamento gerado com sucesso');
                
                // Verificar status para confirmar se o pareamento foi bem-sucedido
                startQrCheck();
                
                return linkingCode;
            }
        }
        
        // Se chegou aqui, houve erro
        const errorMsg = response.error || 'Erro desconhecido ao gerar código';
        addLog(`Falha ao gerar código: ${errorMsg}`, 'error');
        showToast(`Falha ao gerar código: ${errorMsg}`, 'danger');
        return null;
    } catch (error) {
        hideLoading();
        addLog('Erro ao gerar código de pareamento', 'error');
        showToast('Erro ao gerar código de pareamento', 'danger');
        return null;
    }
}

/**
 * Atualizar a UI de status com base nos estados de conexão e login
 * @param {boolean|number} connected - Estado de conexão
 * @param {boolean} loggedIn - Estado de login
 */
function updateStatusUI(connected, loggedIn) {
    // Converter para booleano caso seja um número
    const isConnected = connected === 1 || connected === true;
    
    // Indicadores de status
    const statusIndicator = document.getElementById('statusIndicator');
    const quickStatusIndicator = document.getElementById('quickStatusIndicator');
    const statusText = document.getElementById('statusText');
    const statusTextMobile = document.getElementById('statusTextMobile');
    const instanceStatus = document.getElementById('instanceStatus');
    
    // Atualizar todos os elementos de status
    if (statusIndicator) {
        statusIndicator.className = `status-indicator ${isConnected ? (loggedIn ? 'status-connected' : 'status-waiting') : 'status-disconnected'}`;
    }
    
    if (quickStatusIndicator) {
        quickStatusIndicator.className = `status-indicator ${isConnected ? (loggedIn ? 'status-connected' : 'status-waiting') : 'status-disconnected'}`;
    }
    
    if (statusText) {
        statusText.textContent = isConnected ? (loggedIn ? 'Conectado' : 'Aguardando') : 'Desconectado';
    }
    
    if (statusTextMobile) {
        statusTextMobile.textContent = isConnected ? (loggedIn ? 'Conectado' : 'Aguardando') : 'Desconectado';
    }
    
    if (instanceStatus) {
        instanceStatus.className = `badge ${isConnected ? (loggedIn ? 'bg-success' : 'bg-warning') : 'bg-danger'}`;
        instanceStatus.textContent = isConnected ? (loggedIn ? 'Conectado' : 'Aguardando QR') : 'Desconectado';
    }
    
    // Atualizar a visibilidade dos botões de ação
    if (typeof updateActionButtons === 'function') {
        updateActionButtons();
    } else {
        // Fallback caso a função não esteja disponível
        const actionButtons = document.querySelectorAll('.action-btn');
        
        actionButtons.forEach(button => {
            const condition = button.getAttribute('data-condition');
            
            if (!condition) return;
            
            // Definir visibilidade com base na condição
            switch (condition) {
                case 'disconnected':
                    // Mostrar apenas se desconectado
                    button.classList.toggle('visible', !isConnected);
                    break;
                    
                case 'connected':
                    // Mostrar se conectado (com ou sem login)
                    button.classList.toggle('visible', isConnected);
                    break;
                    
                case 'logged-in':
                    // Mostrar apenas se conectado E logado
                    button.classList.toggle('visible', isConnected && loggedIn);
                    break;
                    
                case 'waiting-qr':
                    // Mostrar se conectado mas não logado (esperando QR)
                    button.classList.toggle('visible', isConnected && !loggedIn);
                    break;
            }
        });
    }
}

/**
 * Obter informações detalhadas sobre uma instância específica
 * @param {string} instanceId - ID da instância
 * @returns {Promise<object>} Informações detalhadas da instância
 */
async function getInstanceInfo(instanceId) {
    if (!instanceId) {
        addLog('ID da instância não fornecido', 'error');
        return null;
    }
    
    try {
        showLoading('Obtendo informações detalhadas da instância...');
        
        // Consultar dados básicos da instância
        const instanceResponse = await makeApiCall(`/instances/${instanceId}`, 'GET');
        
        if (!instanceResponse.success) {
            hideLoading();
            addLog(`Falha ao obter dados da instância ${instanceId}`, 'error');
            return null;
        }
        
        // Consultar status da conexão
        const statusResponse = await makeApiCall(`/instances/${instanceId}/status`, 'GET');
        
        // Consultar informações de webhook
        const webhookResponse = await makeApiCall(`/instances/${instanceId}/webhook`, 'GET');
        
        // Consolidar todas as informações
        const instanceData = instanceResponse.data;
        
        // Adicionar dados de status
        if (statusResponse.success && statusResponse.data) {
            Object.assign(instanceData, statusResponse.data);
        }
        
        // Adicionar dados de webhook
        if (webhookResponse.success && webhookResponse.data) {
            instanceData.webhook = webhookResponse.data.webhook || webhookResponse.data.url;
            instanceData.webhookEvents = webhookResponse.data.subscribe || webhookResponse.data.events;
        }
        
        hideLoading();
        
        // Atualizar a UI com as informações
        updateInstanceDetailUI(instanceData);
        
        return instanceData;
    } catch (error) {
        hideLoading();
        addLog(`Erro ao obter informações da instância ${instanceId}: ${error.message}`, 'error');
        return null;
    }
}

/**
 * Atualiza a UI com informações detalhadas da instância
 * @param {object} instanceData - Dados da instância
 */
function updateInstanceDetailUI(instanceData) {
    const instanceDetailContainer = document.getElementById('instanceDetail');
    if (!instanceDetailContainer) return;
    
    // Determinar status
    const isConnected = instanceData.Connected === true || instanceData.connected === true;
    const isLoggedIn = instanceData.LoggedIn === true || instanceData.loggedIn === true;
    
    let statusClass = 'bg-danger';
    let statusText = 'Desconectado';
    
    if (isConnected) {
        if (isLoggedIn) {
            statusClass = 'bg-success';
            statusText = 'Conectado';
        } else {
            statusClass = 'bg-warning';
            statusText = 'Aguardando QR';
        }
    }
    
    // Formatar eventos webhook
    let eventsHtml = '';
    if (instanceData.webhookEvents) {
        const events = Array.isArray(instanceData.webhookEvents) 
            ? instanceData.webhookEvents 
            : instanceData.webhookEvents.split(',');
            
        eventsHtml = events.map(e => `<span class="badge bg-info me-1">${e}</span>`).join('');
    }
    
    // Construir HTML
    instanceDetailContainer.innerHTML = `
        <div class="card mb-4">
            <div class="card-header">
                <i class="bi bi-info-circle me-2"></i> Detalhes da Instância
            </div>
            <div class="card-body">
                <div class="row">
                    <div class="col-md-6">
                        <h5 class="card-title">${instanceData.name || 'Instância ' + instanceData.id}</h5>
                        <p class="card-text">
                            <strong>ID:</strong> ${instanceData.id}<br>
                            <strong>Status:</strong> <span class="badge ${statusClass}">${statusText}</span><br>
                            <strong>Telefone:</strong> ${instanceData.phone || 'Não conectado'}<br>
                            <strong>JID:</strong> ${instanceData.jid || 'N/A'}<br>
                        </p>
                    </div>
                    <div class="col-md-6">
                        <h5 class="card-title">Configurações</h5>
                        <p class="card-text">
                            <strong>Webhook:</strong> ${instanceData.webhook || 'Não configurado'}<br>
                            <strong>Eventos:</strong> ${eventsHtml || 'Nenhum'}<br>
                            <strong>Proxy:</strong> ${instanceData.proxy_url || 'Não configurado'}<br>
                        </p>
                    </div>
                </div>
                <div class="mt-3">
                    <button class="btn btn-primary btn-sm" onclick="getInstanceInfo('${instanceData.id}')">
                        <i class="bi bi-arrow-clockwise"></i> Atualizar
                    </button>
                    <button class="btn btn-warning btn-sm" onclick="connect()">
                        <i class="bi bi-power"></i> Reconectar
                    </button>
                    <button class="btn btn-danger btn-sm" onclick="deleteInstance('${instanceData.id}')">
                        <i class="bi bi-trash"></i> Excluir
                    </button>
                </div>
            </div>
        </div>
    `;
}

/**
 * Atualizar a lista de instâncias no painel administrativo com botões para mais ações
 * @param {Array} instances - Lista de instâncias
 */
function updateAdminInstancesList(instances) {
    const adminInstancesList = document.getElementById('adminInstancesList');
    if (!adminInstancesList) return;
    
    if (!instances || instances.length === 0) {
        adminInstancesList.innerHTML = '<tr><td colspan="6" class="text-center text-muted">Nenhuma instância encontrada</td></tr>';
        return;
    }
    
    adminInstancesList.innerHTML = '';
    
    instances.forEach((instance, index) => {
        const row = document.createElement('tr');
        row.className = 'fade-in';
        row.style.animationDelay = `${index * 0.05}s`;
        
        const isConnected = instance.connected === 1 || instance.connected === true;
        const isLoggedIn = instance.loggedIn !== undefined ? instance.loggedIn : isConnected;
        
        row.innerHTML = `
            <td>${instance.id}</td>
            <td>${instance.name || 'Instância ' + instance.id}</td>
            <td>${instance.phone || 'Não conectada'}</td>
            <td>
                <span class="badge ${isConnected ? (isLoggedIn ? 'bg-success' : 'bg-warning') : 'bg-danger'}">
                    ${isConnected ? (isLoggedIn ? 'Conectada' : 'Aguardando QR') : 'Desconectada'}
                </span>
            </td>
            <td>${instance.webhook ? '<span class="badge bg-info">Configurado</span>' : '<span class="badge bg-secondary">Não configurado</span>'}</td>
            <td>
                <div class="btn-group btn-group-sm">
                    <button class="btn btn-outline-primary btn-sm admin-connect-btn" data-id="${instance.id}" title="Conectar">
                        <i class="bi bi-power"></i>
                    </button>
                    <button class="btn btn-outline-danger btn-sm admin-disconnect-btn" data-id="${instance.id}" title="Desconectar">
                        <i class="bi bi-plug"></i>
                    </button>
                    <button class="btn btn-outline-secondary btn-sm admin-test-btn" data-id="${instance.id}" title="Enviar teste">
                        <i class="bi bi-chat-dots"></i>
                    </button>
                    <button class="btn btn-outline-info btn-sm admin-info-btn" data-id="${instance.id}" title="Informações detalhadas">
                        <i class="bi bi-info-circle"></i>
                    </button>
                    <button class="btn btn-outline-danger btn-sm admin-delete-btn" data-id="${instance.id}" title="Excluir completamente">
                        <i class="bi bi-trash"></i>
                    </button>
                </div>
            </td>
        `;
        
        adminInstancesList.appendChild(row);
    });
    
    // Adicionar eventos aos botões de ação
    adminInstancesList.querySelectorAll('.admin-connect-btn').forEach(btn => {
        btn.addEventListener('click', () => {
            const instanceId = btn.getAttribute('data-id');
            if (instanceId) {
                connectInstance(instanceId).then(() => {
                    showToast('Solicitação de conexão enviada', 'success');
                    setTimeout(() => loadAdminStats(), 1500);
                });
            }
        });
    });
    
    adminInstancesList.querySelectorAll('.admin-disconnect-btn').forEach(btn => {
        btn.addEventListener('click', () => {
            const instanceId = btn.getAttribute('data-id');
            if (instanceId) {
                disconnectInstance(instanceId).then(() => {
                    showToast('Instância desconectada', 'warning');
                    setTimeout(() => loadAdminStats(), 1500);
                });
            }
        });
    });
    
    adminInstancesList.querySelectorAll('.admin-test-btn').forEach(btn => {
        btn.addEventListener('click', () => {
            const instanceId = btn.getAttribute('data-id');
            if (instanceId) {
                // Selecionar a instância no dropdown e rolar até o formulário
                const adminTestInstance = document.getElementById('adminTestInstance');
                if (adminTestInstance) {
                    adminTestInstance.value = instanceId;
                    adminTestInstance.scrollIntoView({ behavior: 'smooth', block: 'center' });
                    // Destacar o campo de mensagem
                    const adminTestMessage = document.getElementById('adminTestMessage');
                    if (adminTestMessage) {
                        adminTestMessage.focus();
                        adminTestMessage.classList.add('is-valid');
                        setTimeout(() => adminTestMessage.classList.remove('is-valid'), 1500);
                    }
                }
            }
        });
    });
    
    // Adicionar evento para o botão de informações detalhadas
    adminInstancesList.querySelectorAll('.admin-info-btn').forEach(btn => {
        btn.addEventListener('click', () => {
            const instanceId = btn.getAttribute('data-id');
            if (instanceId) {
                getInstanceInfo(instanceId).then(instanceData => {
                    if (instanceData) {
                        // Mostrar modal com informações detalhadas
                        const modal = new bootstrap.Modal(document.getElementById('instanceInfoModal') || createInstanceInfoModal());
                        modal.show();
                    }
                });
            }
        });
    });
    
    // Adicionar evento para o botão de exclusão completa
    adminInstancesList.querySelectorAll('.admin-delete-btn').forEach(btn => {
        btn.addEventListener('click', () => {
            const instanceId = btn.getAttribute('data-id');
            if (instanceId) {
                if (confirm(`Tem certeza que deseja excluir COMPLETAMENTE a instância ${instanceId}? Esta ação não pode ser desfeita e irá remover todos os dados, incluindo sessões e arquivos.`)) {
                    deleteInstance(instanceId).then(success => {
                        if (success) {
                            loadAdminStats();
                        }
                    });
                }
            }
        });
    });
}

/**
 * Cria o modal de informações detalhadas da instância se ele não existir
 * @returns {HTMLElement} O elemento do modal
 */
function createInstanceInfoModal() {
    const modalHTML = `
        <div class="modal fade" id="instanceInfoModal" tabindex="-1" aria-labelledby="instanceInfoModalLabel" aria-hidden="true">
            <div class="modal-dialog modal-lg">
                <div class="modal-content">
                    <div class="modal-header">
                        <h5 class="modal-title" id="instanceInfoModalLabel">Informações Detalhadas da Instância</h5>
                        <button type="button" class="btn-close" data-bs-dismiss="modal" aria-label="Fechar"></button>
                    </div>
                    <div class="modal-body">
                        <div id="instanceDetail"></div>
                    </div>
                    <div class="modal-footer">
                        <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">Fechar</button>
                    </div>
                </div>
            </div>
        </div>
    `;
    
    const modalElement = document.createElement('div');
    modalElement.innerHTML = modalHTML;
    document.body.appendChild(modalElement.firstChild);
    
    return document.getElementById('instanceInfoModal');
}

// Inicialização
document.addEventListener('DOMContentLoaded', () => {
    // Registrar eventos para os botões de instância
    const createInstanceBtn = document.getElementById('createInstanceBtn');
    if (createInstanceBtn) {
        createInstanceBtn.addEventListener('click', () => {
            const newInstanceName = document.getElementById('newInstanceName');
            const newInstanceToken = document.getElementById('newInstanceToken');
            const newInstanceWebhook = document.getElementById('newInstanceWebhook');
            const newInstanceProxy = document.getElementById('newInstanceProxy');
            const newEnableProxyCheck = document.getElementById('newEnableProxyCheck');
            
            if (!newInstanceName) return;
            
            const name = newInstanceName.value.trim();
            if (!name) {
                showToast('Informe um nome para a instância', 'warning');
                return;
            }
            
            // Coletar eventos selecionados
            const events = [];
            document.querySelectorAll('.new-instance-event:checked').forEach(checkbox => {
                events.push(checkbox.value);
            });
            
            // Verificar se "Todos os Eventos" está marcado
            if (document.getElementById('newEventAll')?.checked) {
                events.push('All');
            }
            
            // Preparar dados da instância
            const instanceData = {
                name: name,
                token: newInstanceToken?.value?.trim() || '',
            };
            
            // Adicionar webhook se fornecido
            if (newInstanceWebhook && newInstanceWebhook.value.trim()) {
                instanceData.webhook = newInstanceWebhook.value.trim();
                instanceData.events = events;
            }
            
            // Adicionar proxy se fornecido e habilitado
            if (newInstanceProxy && newInstanceProxy.value.trim() && newEnableProxyCheck?.checked) {
                instanceData.proxyUrl = newInstanceProxy.value.trim();
                instanceData.proxyEnabled = true;
            }
            
            createInstance(instanceData);
        });
    }
    
    // Event listener para o botão de gerar token
    const generateTokenBtn = document.getElementById('generateTokenBtn');
    if (generateTokenBtn) {
        generateTokenBtn.addEventListener('click', () => {
            const newInstanceToken = document.getElementById('newInstanceToken');
            if (newInstanceToken) {
                newInstanceToken.value = generateInstanceToken();
            }
        });
    }
    
    // Event listener para salvar nome da instância
    const saveInstanceLabelBtn = document.getElementById('saveInstanceLabelBtn');
    if (saveInstanceLabelBtn) {
        saveInstanceLabelBtn.addEventListener('click', () => {
            if (!instancesState.currentInstance) {
                showToast('Selecione uma instância primeiro', 'warning');
                return;
            }
            
            const instanceLabel = document.getElementById('instanceLabel');
            if (!instanceLabel) return;
            
            const name = instanceLabel.value.trim();
            if (!name) {
                showToast('Informe um nome para a instância', 'warning');
                return;
            }
            
            updateInstanceName(instancesState.currentInstance, name);
        });
    }
    
    // Event listener para confirmar exclusão de instância
    const confirmDeleteBtn = document.getElementById('confirmDeleteBtn');
    if (confirmDeleteBtn) {
        confirmDeleteBtn.addEventListener('click', () => {
            if (!instancesState.currentInstance) {
                showToast('Nenhuma instância selecionada', 'warning');
                return;
            }
            
            deleteInstance(instancesState.currentInstance);
        });
    }
    
    // Adicionar eventos aos botões de controle de instância
    const connectBtn = document.getElementById('connectBtn');
    if (connectBtn) {
        connectBtn.addEventListener('click', connect);
    }
    
    const disconnectBtn = document.getElementById('disconnectBtn');
    if (disconnectBtn) {
        disconnectBtn.addEventListener('click', disconnect);
    }
    
    const logoutBtn = document.getElementById('logoutBtn');
    if (logoutBtn) {
        logoutBtn.addEventListener('click', logout);
    }
    
    const refreshStatus = document.getElementById('refreshStatus');
    if (refreshStatus) {
        refreshStatus.addEventListener('click', checkStatus);
    }
    
    const regenerateQrBtn = document.getElementById('regenerateQrBtn');
    if (regenerateQrBtn) {
        regenerateQrBtn.addEventListener('click', getQrCode);
    }
    
    const syncContactsBtn = document.getElementById('syncContactsBtn');
    if (syncContactsBtn) {
        syncContactsBtn.addEventListener('click', syncContacts);
    }
    
    const checkSystemStatusBtn = document.getElementById('checkSystemStatusBtn');
    if (checkSystemStatusBtn) {
        checkSystemStatusBtn.addEventListener('click', async () => {
            const report = await checkSystemStatus();
            if (!report) return;
            
            // Mostrar relatório em um modal
            const modalHtml = `
                <div class="modal fade" id="systemStatusModal" tabindex="-1" aria-hidden="true">
                    <div class="modal-dialog">
                        <div class="modal-content">
                            <div class="modal-header">
                                <h5 class="modal-title">Status do Sistema</h5>
                                <button type="button" class="btn-close" data-bs-dismiss="modal" aria-label="Close"></button>
                            </div>
                            <div class="modal-body">
                                <pre class="bg-light p-3 rounded" style="white-space: pre-wrap;">${JSON.stringify(report, null, 2)}</pre>
                            </div>
                            <div class="modal-footer">
                                <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">Fechar</button>
                            </div>
                        </div>
                    </div>
                </div>
            `;
            
            // Adicionar modal ao corpo do documento
            const modalContainer = document.createElement('div');
            modalContainer.innerHTML = modalHtml;
            document.body.appendChild(modalContainer);
            
            // Mostrar modal
            const modal = new bootstrap.Modal(document.getElementById('systemStatusModal'));
            modal.show();
            
            // Remover modal do DOM quando fechado
            document.getElementById('systemStatusModal').addEventListener('hidden.bs.modal', () => {
                modalContainer.remove();
            });
        });
    }
    
    // Botões de ação rápida
    const quickConnectBtn = document.getElementById('quickConnectBtn');
    if (quickConnectBtn) {
        quickConnectBtn.addEventListener('click', connect);
    }
    
    const quickDisconnectBtn = document.getElementById('quickDisconnectBtn');
    if (quickDisconnectBtn) {
        quickDisconnectBtn.addEventListener('click', disconnect);
    }
    
    const quickLogoutBtn = document.getElementById('quickLogoutBtn');
    if (quickLogoutBtn) {
        quickLogoutBtn.addEventListener('click', logout);
    }
    
    const quickQrBtn = document.getElementById('quickQrBtn');
    if (quickQrBtn) {
        quickQrBtn.addEventListener('click', getQrCode);
    }
    
    // Restaurar estado da conexão
    restoreConnectionState();
});

/**
 * Verifica apenas o status da instância atual para usuários não-admin
 * @returns {Promise<object>} Status da instância
 */
async function checkInstanceStatus() {
    if (!instancesState.currentInstance) {
        addLog('Nenhuma instância selecionada', 'error');
        showToast('Selecione uma instância primeiro', 'warning');
        return null;
    }
    
    try {
        showLoading('Verificando status da instância...');
        addLog(`Verificando status da instância ${instancesState.currentInstance}...`);
        
        // Consultar status da instância
        const statusResponse = await makeApiCall(`/instances/${instancesState.currentInstance}/status`, 'GET');
        
        // Consultar informações de webhook
        const webhookResponse = await makeApiCall(`/instances/${instancesState.currentInstance}/webhook`, 'GET');
        
        hideLoading();
        
        if (!statusResponse.success) {
            const errorMsg = statusResponse.error || 'Erro desconhecido';
            addLog(`Falha ao verificar status: ${errorMsg}`, 'error');
            showToast(`Erro ao verificar status: ${errorMsg}`, 'danger');
            return null;
        }
        
        // Informações da instância
        const instance = instancesState.instances.find(i => i.id === instancesState.currentInstance);
        const instanceName = instance ? instance.name || `Instância ${instance.id}` : instancesState.currentInstance;
        
        // Determinar status
        const isConnected = statusResponse.data?.connected === true || 
                          statusResponse.data?.Connected === true;
        
        const isLoggedIn = statusResponse.data?.loggedIn === true || 
                         statusResponse.data?.LoggedIn === true;
        
        const phoneNumber = statusResponse.data?.phone || 
                          (instance ? instance.phone : '');
        
        // Montar informações de webhook
        let webhookInfo = 'Não configurado';
        let webhookEvents = [];
        
        if (webhookResponse.success && webhookResponse.data) {
            if (webhookResponse.data.webhook || webhookResponse.data.url) {
                webhookInfo = webhookResponse.data.webhook || webhookResponse.data.url;
            }
            
            if (webhookResponse.data.subscribe || webhookResponse.data.events) {
                const events = webhookResponse.data.subscribe || webhookResponse.data.events;
                webhookEvents = Array.isArray(events) ? events : events.split(',');
            }
        }
        
        // Montar resultados
        const results = {
            success: true,
            instanceId: instancesState.currentInstance,
            instanceName: instanceName,
            status: {
                connected: isConnected,
                loggedIn: isLoggedIn,
                phoneNumber: phoneNumber,
                statusText: isConnected 
                    ? (isLoggedIn ? 'Conectado' : 'Aguardando QR')
                    : 'Desconectado'
            },
            webhook: {
                url: webhookInfo,
                events: webhookEvents
            }
        };
        
        addLog('Verificação de status da instância concluída', 'success');
        
        // Mostrar resultados em um modal
        const modalHtml = `
            <div class="modal fade" id="instanceStatusModal" tabindex="-1" aria-hidden="true">
                <div class="modal-dialog">
                    <div class="modal-content">
                        <div class="modal-header">
                            <h5 class="modal-title">Status da Instância</h5>
                            <button type="button" class="btn-close" data-bs-dismiss="modal" aria-label="Close"></button>
                        </div>
                        <div class="modal-body">
                            <div class="mb-4">
                                <h6 class="border-bottom pb-2">Informações Gerais</h6>
                                <div class="row mb-2">
                                    <div class="col-5 fw-bold">Nome:</div>
                                    <div class="col-7">${results.instanceName}</div>
                                </div>
                                <div class="row mb-2">
                                    <div class="col-5 fw-bold">ID:</div>
                                    <div class="col-7">${results.instanceId}</div>
                                </div>
                                <div class="row mb-2">
                                    <div class="col-5 fw-bold">Status:</div>
                                    <div class="col-7">
                                        <span class="badge ${isConnected ? (isLoggedIn ? 'bg-success' : 'bg-warning') : 'bg-danger'}">
                                            ${results.status.statusText}
                                        </span>
                                    </div>
                                </div>
                                <div class="row mb-2">
                                    <div class="col-5 fw-bold">Telefone:</div>
                                    <div class="col-7">${results.status.phoneNumber || 'Não conectado'}</div>
                                </div>
                            </div>
                            
                            <div>
                                <h6 class="border-bottom pb-2">Webhook</h6>
                                <div class="row mb-2">
                                    <div class="col-5 fw-bold">URL:</div>
                                    <div class="col-7 text-break">${results.webhook.url}</div>
                                </div>
                                <div class="row mb-2">
                                    <div class="col-5 fw-bold">Eventos:</div>
                                    <div class="col-7">
                                        ${results.webhook.events.length > 0 
                                            ? results.webhook.events.map(e => `<span class="badge bg-info me-1">${e}</span>`).join('') 
                                            : 'Nenhum evento configurado'}
                                    </div>
                                </div>
                            </div>
                        </div>
                        <div class="modal-footer">
                            <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">Fechar</button>
                            <button type="button" class="btn btn-primary" onclick="checkInstanceStatus()">
                                <i class="bi bi-arrow-clockwise"></i> Atualizar
                            </button>
                        </div>
                    </div>
                </div>
            </div>
        `;
        
        // Adicionar modal ao corpo do documento
        const modalContainer = document.createElement('div');
        modalContainer.innerHTML = modalHtml;
        document.body.appendChild(modalContainer);
        
        // Mostrar modal
        const modal = new bootstrap.Modal(document.getElementById('instanceStatusModal'));
        modal.show();
        
        // Remover modal do DOM quando fechado
        document.getElementById('instanceStatusModal').addEventListener('hidden.bs.modal', () => {
            modalContainer.remove();
        });
        
        return results;
    } catch (error) {
        hideLoading();
        addLog(`Erro ao verificar status da instância: ${error.message}`, 'error');
        showToast('Erro ao verificar status', 'danger');
        return null;
    }
}

/**
 * Restaurar estado da conexão após recarregar a página
 */
async function restoreConnectionState() {
    const savedInstanceId = localStorage.getItem('currentInstance');
    const savedStatus = localStorage.getItem('instanceStatus');
    
    if (savedInstanceId && savedStatus) {
        try {
            const status = JSON.parse(savedStatus);
            const lastCheck = status.lastCheck || 0;
            const now = Date.now();
            
            // Se o último check foi há menos de 1 minuto, usar o estado salvo
            if (now - lastCheck < 60000 && status.connected) {
                instancesState.currentInstance = savedInstanceId;
                updateStatusUI(status.connected, status.loggedIn);
                startStatusPolling();
                
                // Verificar status silenciosamente para atualizar
                checkStatusSilently(savedInstanceId);
            } else {
                // Se passou muito tempo, verificar status normalmente
                checkStatus(savedInstanceId);
            }
        } catch (error) {
            console.error('Erro ao restaurar estado da conexão:', error);
            // Em caso de erro, verificar status normalmente
            if (savedInstanceId) {
                checkStatus(savedInstanceId);
            }
        }
    }
}

/**
 * Conectar a instância atual
 * @returns {Promise<boolean>} Sucesso da operação
 */
async function connect() {
    if (!instancesState.currentInstance) {
        addLog('Nenhuma instância selecionada', 'error');
        showToast('Selecione uma instância primeiro', 'warning');
        return false;
    }
    
    try {
        const instanceId = instancesState.currentInstance;
        showLoading('Conectando instância ' + instanceId + '...');
        addLog(`Iniciando conexão para instância ${instanceId}...`);
        
        // Verificar status atual antes de conectar
        const statusCheck = await makeApiCall(`/instances/${instanceId}/status`, 'GET');
        let needsClear = false;
        
        // Se houver erro de sessão ou status 500, pode ser necessário limpar a sessão anterior
        if (!statusCheck.success || 
            (statusCheck.data && statusCheck.data.noSession) || 
            (statusCheck.data && statusCheck.data.isOfflineFallback)) {
            needsClear = true;
            addLog('Detectado problema na sessão anterior. Preparando reconexão...', 'warning');
        }
        
        // Usar o formato correto de payload como em login.html
        const payload = {
            Subscribe: ["Message", "ReadReceipt", "Presence", "ChatPresence"],
            Immediate: true,
            // Se precisar limpar a sessão, adicionamos o flag para isso
            ClearSession: needsClear
        };
        
        // Recuperar eventos selecionados se houver
        const eventCheckboxes = document.querySelectorAll('input[type=checkbox][id^=event]:checked');
        if (eventCheckboxes.length > 0) {
            payload.Subscribe = [];
            eventCheckboxes.forEach(checkbox => {
                payload.Subscribe.push(checkbox.value);
            });
        }
        
        // Usar o ID específico da instância na URL
        const response = await makeApiCall(`/instances/${instanceId}/connect`, 'POST', payload);
        
        hideLoading();
        
        if (response.success) {
            addLog(`Conexão iniciada com sucesso para instância ${instanceId}`, 'success');
            showToast('Conexão iniciada');
            
            // Atualizar a instância atual com os dados recebidos
            const instance = instancesState.instances.find(i => i.id === instanceId);
            if (instance) {
                // Primeiramente, marcamos como conectado mas não logado
                instance.connected = 1;
                instance.isConnected = true;
                instance.loggedIn = false;
                instance.isLoggedIn = false;
                
                // Atualizar dados da instância com os retornados pela API
                if (response.data) {
                    // Processar número de telefone do JID se disponível
                    if (response.data.jid) {
                        instance.jid = response.data.jid;
                        instance.phone = extractPhoneFromJid(response.data.jid);
                        
                        const phoneNumber = document.getElementById('phoneNumber');
                        if (phoneNumber) {
                            phoneNumber.textContent = instance.phone;
                        }
                    }
                    
                    // Processar webhook se disponível
                    if (response.data.webhook) {
                        instance.webhook = response.data.webhook;
                        
                        const webhookUrl = document.getElementById('webhookUrl');
                        const webhookUrlInput = document.getElementById('webhookUrlInput');
                        if (webhookUrl) webhookUrl.textContent = response.data.webhook;
                        if (webhookUrlInput) webhookUrlInput.value = response.data.webhook;
                    }
                    
                    // Processar eventos se disponíveis
                    if (response.data.events) {
                        let eventsArray;
                        if (typeof response.data.events === 'string') {
                            eventsArray = response.data.events.split(',');
                            instance.events = response.data.events;
                        } else if (Array.isArray(response.data.events)) {
                            eventsArray = response.data.events;
                            instance.events = response.data.events.join(',');
                        } else {
                            eventsArray = [];
                            instance.events = '';
                        }
                        
                        instance.eventsArray = eventsArray;
                        
                        const subscribedEvents = document.getElementById('subscribedEvents');
                        if (subscribedEvents) {
                            subscribedEvents.textContent = Array.isArray(eventsArray) 
                                ? eventsArray.join(', ') 
                                : eventsArray;
                        }
                    }
                }
                
                // Atualizar UI
                renderInstanceList();
                renderMobileInstanceList();
                updateStatusUI(true, false);
            }
            
            // Verificar se precisa mostrar QR - esperar um momento para o servidor processar a conexão
            setTimeout(async () => {
                // Verificar status diretamente na API para esta instância específica
                const statusResponse = await makeApiCall(`/instances/${instanceId}/status`, 'GET');
                
                let isConnected = false;
                let isLoggedIn = false;
                
                if (statusResponse.success) {
                    // Verificar ambos os formatos possíveis de resposta
                    if (statusResponse.data.hasOwnProperty('Connected')) {
                        isConnected = statusResponse.data.Connected === true;
                        isLoggedIn = statusResponse.data.LoggedIn === true;
                    } else if (statusResponse.data.hasOwnProperty('connected')) {
                        isConnected = statusResponse.data.connected === true;
                        isLoggedIn = statusResponse.data.loggedIn === true;
                    }
                    
                    if (isConnected && !isLoggedIn) {
                        // Se conectado mas não logado, mostrar QR code
                        getQrCode();
                    } else if (isConnected && isLoggedIn) {
                        // Se já estiver logado, atualizar UI
                        updateStatusUI(true, true);
                    }
                } else {
                    // Se houver erro, tentar obter QR code de qualquer forma
                    getQrCode();
                }
            }, 1500);
            
            return true;
        } else {
            // Tratar erro específico "no session"
            if (response.error && response.error.toLowerCase().includes('no session')) {
                addLog(`Erro de sessão na instância ${instanceId}. Tente novamente para criar uma nova sessão.`, 'warning');
                showToast('Instância sem sessão ativa. Tente novamente.', 'warning');
            } else {
                addLog(`Falha ao conectar instância ${instanceId}: ${response.error || 'Erro desconhecido'}`, 'error');
                showToast(`Falha ao conectar: ${response.error || 'Erro desconhecido'}`, 'danger');
            }
            
            // Marcar instância como desconectada na UI
            updateInstanceInList(instanceId, false, false, '');
            updateStatusUI(false, false);
            
            return false;
        }
    } catch (error) {
        hideLoading();
        addLog('Erro ao tentar conectar', 'error');
        showToast('Erro ao tentar conectar', 'danger');
        return false;
    }
}

/**
 * Desconectar a instância atual
 * @returns {Promise<boolean>} Sucesso da operação
 */
async function disconnect() {
    if (!instancesState.currentInstance) {
        addLog('Nenhuma instância selecionada', 'error');
        showToast('Selecione uma instância primeiro', 'warning');
        return false;
    }
    
    try {
        const instanceId = instancesState.currentInstance;
        showLoading(`Desconectando instância ${instanceId}...`);
        addLog(`Iniciando desconexão para instância ${instanceId}...`);
        
        // Chamada direta à API especificando a instância correta
        const response = await makeApiCall(`/instances/${instanceId}/disconnect`, 'POST');
        
        hideLoading();
        
        if (response.success) {
            addLog(`Instância ${instanceId} desconectada com sucesso`, 'success');
            showToast('Desconectado com sucesso');
            clearQrCheck();
            
            // Atualizar na lista de instâncias
            const instance = instancesState.instances.find(i => i.id === instanceId);
            if (instance) {
                instance.connected = 0;
                instance.isConnected = false;
                instance.loggedIn = false;
                instance.isLoggedIn = false;
                
                // Atualizar UI
                renderInstanceList();
                renderMobileInstanceList();
                
                // Atualizar UI da instância atual
                if (instancesState.currentInstance === instanceId) {
                    updateStatusUI(false, false);
                    
                    // Limpar QR code
                    const qrCode = document.getElementById('qrCode');
                    if (qrCode) {
                        qrCode.innerHTML = '<p class="text-muted">O código QR será exibido aqui quando você conectar.</p>';
                    }
                }
            }
            
            return true;
        } else {
            addLog(`Falha ao desconectar instância ${instanceId}: ${response.error || 'Erro desconhecido'}`, 'error');
            showToast(`Falha ao desconectar: ${response.error || 'Erro desconhecido'}`, 'danger');
            return false;
        }
    } catch (error) {
        hideLoading();
        addLog('Erro ao tentar desconectar', 'error');
        showToast('Erro ao tentar desconectar', 'danger');
        return false;
    }
}

/**
 * Logout da instância atual (encerra a sessão)
 * @returns {Promise<boolean>} Sucesso da operação
 */
async function logout() {
    if (!instancesState.currentInstance) {
        addLog('Nenhuma instância selecionada', 'error');
        showToast('Selecione uma instância primeiro', 'warning');
        return false;
    }
    
    try {
        const instanceId = instancesState.currentInstance;
        showLoading(`Encerrando sessão da instância ${instanceId}...`);
        addLog(`Encerrando sessão para instância ${instanceId}...`);
        
        // Chamada direta à API especificando a instância correta
        const response = await makeApiCall(`/instances/${instanceId}/logout`, 'POST');
        
        hideLoading();
        
        if (response.success) {
            addLog(`Sessão da instância ${instanceId} encerrada com sucesso`, 'success');
            showToast('Sessão encerrada com sucesso');
            clearQrCheck();
            
            // Atualizar instância na lista
            const instance = instancesState.instances.find(i => i.id === instanceId);
            if (instance) {
                // Após logout, a instância ainda está conectada, mas não logada
                instance.connected = 1;
                instance.isConnected = true;
                instance.loggedIn = false;
                instance.isLoggedIn = false;
                
                // Atualizar UI
                renderInstanceList();
                renderMobileInstanceList();
                
                // Atualizar UI da instância atual
                if (instancesState.currentInstance === instanceId) {
                    updateStatusUI(true, false);
                    
                    // Atualizar QR code
                    const qrCode = document.getElementById('qrCode');
                    if (qrCode) {
                        qrCode.innerHTML = '<p class="text-muted">O código QR será exibido aqui quando você conectar.</p>';
                    }
                    
                    // Gerar novo QR code após logout (aguardar um pouco para o servidor processar)
                    setTimeout(() => {
                        getQrCode();
                    }, 1500);
                }
            }
            
            return true;
        } else {
            addLog(`Falha ao encerrar sessão da instância ${instanceId}: ${response.error || 'Erro desconhecido'}`, 'error');
            showToast(`Falha ao encerrar sessão: ${response.error || 'Erro desconhecido'}`, 'danger');
            return false;
        }
    } catch (error) {
        hideLoading();
        addLog('Erro ao tentar encerrar sessão', 'error');
        showToast('Erro ao tentar encerrar sessão', 'danger');
        return false;
    }
}

/**
 * Conectar a instância atual
 * @returns {Promise<boolean>} Sucesso da operação
 */
async function connectInstance(instanceId) {
    if (!instanceId) {
        addLog('ID da instância não fornecido', 'error');
        showToast('Selecione uma instância primeiro', 'warning');
        return false;
    }
    
    try {
        const instance = instancesState.instances.find(i => i.id === instanceId);
        if (!instance) {
            addLog('Instância não encontrada', 'error');
            showToast('Instância não encontrada', 'danger');
            return false;
        }
        
        // Salvar instância atual
        const currentInstanceBkp = instancesState.currentInstance;
        
        // Selecionar temporariamente a instância para usar connect()
        instancesState.currentInstance = instanceId;
        
        const result = await connect();
        
        // Restaurar instância anterior
        instancesState.currentInstance = currentInstanceBkp;
        
        return result;
    } catch (error) {
        hideLoading();
        addLog('Erro ao tentar conectar instância', 'error');
        showToast('Erro ao tentar conectar instância', 'danger');
        return false;
    }
}

/**
 * Desconectar uma instância específica
 * @param {string} instanceId - ID da instância
 * @returns {Promise<boolean>} Sucesso da operação
 */
async function disconnectInstance(instanceId) {
    if (!instanceId) {
        addLog('ID da instância não fornecido', 'error');
        showToast('Selecione uma instância primeiro', 'warning');
        return false;
    }
    
    try {
        const instance = instancesState.instances.find(i => i.id === instanceId);
        if (!instance) {
            addLog('Instância não encontrada', 'error');
            showToast('Instância não encontrada', 'danger');
            return false;
        }
        
        // Salvar instância atual
        const currentInstanceBkp = instancesState.currentInstance;
        
        // Selecionar temporariamente a instância para usar disconnect()
        instancesState.currentInstance = instanceId;
        
        const result = await disconnect();
        
        // Restaurar instância anterior
        instancesState.currentInstance = currentInstanceBkp;
        
        return result;
    } catch (error) {
        hideLoading();
        addLog('Erro ao tentar desconectar instância', 'error');
        showToast('Erro ao tentar desconectar instância', 'danger');
        return false;
    }
}

/**
 * Logout de uma instância específica
 * @param {string} instanceId - ID da instância
 * @returns {Promise<boolean>} Sucesso da operação
 */
async function logoutInstance(instanceId) {
    if (!instanceId) {
        addLog('ID da instância não fornecido', 'error');
        showToast('Selecione uma instância primeiro', 'warning');
        return false;
    }
    
    try {
        const instance = instancesState.instances.find(i => i.id === instanceId);
        if (!instance) {
            addLog('Instância não encontrada', 'error');
            showToast('Instância não encontrada', 'danger');
            return false;
        }
        
        // Salvar instância atual
        const currentInstanceBkp = instancesState.currentInstance;
        
        // Selecionar temporariamente a instância
        instancesState.currentInstance = instanceId;
        
        const result = await logout();
        
        // Restaurar instância anterior
        instancesState.currentInstance = currentInstanceBkp;
        
        return result;
    } catch (error) {
        hideLoading();
        addLog('Erro ao tentar encerrar sessão', 'error');
        showToast('Erro ao tentar encerrar sessão', 'danger');
        return false;
    }
} 