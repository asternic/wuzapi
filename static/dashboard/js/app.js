// Main Application Module

// Configurações globais
let appConfig = {
    debug: false,
    version: '1.1.0',
    reconnectAttempts: 0,
    maxReconnectAttempts: 3,
    reconnectDelay: 5000, // 5 segundos
    adminReconnectDelay: 10000, // 10 segundos para admin
    isReconnecting: false,
    hasTriedSessionReconnect: false
};

// Estado global da aplicação
let appState = {
    statusPollingInterval: null,
    isPolling: false,
    lastStatus: null,
    qrCheckInterval: null,
    instanceDashboard: null
};

/**
 * Função para sincronizar o token de autenticação entre diferentes locais de armazenamento
 * Isso evita problemas de autenticação quando o token está presente em um local mas não em outro
 */
function syncAuthToken() {
    console.log("[AUTH] Sincronizando token de autenticação entre fontes...");
    
    // Verificar token na URL
    const urlParams = new URLSearchParams(window.location.search);
    const urlToken = urlParams.get('token');
    
    // Verificar token no localStorage
    const localStorageToken = localStorage.getItem('wuzapiToken');
    
    // Verificar token na window
    const windowToken = window.userToken;
    
    // Verificar token no estado de autenticação
    const authStateToken = window.authState?.userToken;
    
    // Logs para diagnóstico
    console.log(`[AUTH] Token na URL: ${urlToken ? 'Sim' : 'Não'}`);
    console.log(`[AUTH] Token no localStorage: ${localStorageToken ? 'Sim' : 'Não'}`);
    console.log(`[AUTH] Token em window.userToken: ${windowToken ? 'Sim' : 'Não'}`);
    console.log(`[AUTH] Token em authState: ${authStateToken ? 'Sim' : 'Não'}`);
    
    // Determinar o token mais confiável, na ordem: URL > localStorage > window > authState
    let finalToken = urlToken || localStorageToken || windowToken || authStateToken || '';
    
    if (!finalToken) {
        console.warn("[AUTH] Atenção: Nenhum token encontrado em nenhuma fonte");
        return false;
    }
    
    console.log(`[AUTH] Token final selecionado: ${finalToken.substring(0, 10)}...`);
    
    // Atualizar todas as fontes com o token final
    if (finalToken) {
        // Atualizar window.userToken
        window.userToken = finalToken;
        
        // Atualizar localStorage
        localStorage.setItem('wuzapiToken', finalToken);
        
        // Atualizar authState se existir
        if (window.authState) {
            window.authState.userToken = finalToken;
            window.authState.isAuthenticated = true;
        } else {
            // Criar authState se não existir
            window.authState = {
                userToken: finalToken,
                isAuthenticated: true,
                isAdmin: false,
                userId: null,
                userName: ''
            };
        }
        
        // Atualizar URL se não tiver o token
        if (urlToken !== finalToken) {
            const newUrl = new URL(window.location.href);
            newUrl.searchParams.set('token', finalToken);
            window.history.replaceState({}, '', newUrl.toString());
            console.log("[AUTH] Token adicionado à URL");
        }
        
        console.log("[AUTH] Token sincronizado com sucesso em todas as fontes");
        return true;
    }
    
    return false;
}

/**
 * Inicializar a interface do usuário
 */
function initializeInterface() {
    // Verificar autenticação primeiro
    if (!authState.isAuthenticated) {
        return;
    }

    // Inicializar tabs e outras funcionalidades do Bootstrap
    document.querySelectorAll('[data-bs-toggle="tooltip"]').forEach(el => {
        new bootstrap.Tooltip(el);
    });
    
    // Verificar tipo de usuário
    const isAdmin = authState?.isAdmin === true;
    
    // Configurações específicas por tipo de usuário
    if (isAdmin) {
        // Inicializar interface para admin
        initAdminInterface();
    } else {
        // Inicializar interface para usuário normal
        initUserInterface();
    }
    
    // Registrar eventos comuns
    registerCommonEvents();
    
    // Aplicar restrições de interface
    applyUserInterfaceRestrictions();
    
    // Mostrar toast de boas vindas
    setTimeout(() => {
        showToast(`Bem-vindo ao WuzAPI Manager, ${authState.userName || 'Usuário'}!`, 'primary');
    }, 1000);
    
    // Restaurar estado da instância após recarregar a página
    restoreInstanceState();
    
    // Verificar a visibilidade dos botões de ação
    updateActionButtons();
}

/**
 * Inicializar interface específica para administradores
 */
function initAdminInterface() {
    // Mostrar elementos específicos de admin
    document.querySelectorAll('.admin-only').forEach(el => {
        el.style.display = '';
    });
    
    // Mostrar também elementos específicos de usuário para permitir gerenciamento de instâncias
    document.querySelectorAll('.user-only').forEach(el => {
        el.style.display = '';
    });
    
    // Mostrar o badge de admin
    const adminBadge = document.getElementById('adminBadge');
    if (adminBadge) {
        adminBadge.classList.remove('d-none');
    }
    
    // Inicializar o painel de administração
    initAdminPanel();
    
    // Garantir que a sidebar de instâncias seja visível
    const instanceSidebar = document.querySelector('.col-md-3.col-lg-2');
    if (instanceSidebar) {
        instanceSidebar.classList.remove('d-none');
    }
    
    // Expandir o conteúdo principal
    const mainContent = document.querySelector('.col-md-9.col-lg-10');
    if (mainContent) {
        mainContent.classList.remove('col-12');
        mainContent.classList.add('col-md-9', 'col-lg-10');
    }
    
    addLog('Painel de administração ativado', 'info');
}

/**
 * Inicializar interface específica para usuários normais
 */
function initUserInterface() {
    // Ocultar elementos específicos de admin
    document.querySelectorAll('.admin-only').forEach(el => {
        el.style.display = 'none';
    });
    
    // Mostrar elementos específicos de usuário
    document.querySelectorAll('.user-only').forEach(el => {
        el.style.display = '';
    });
    
    // Esconder o badge de admin
    const adminBadge = document.getElementById('adminBadge');
    if (adminBadge) {
        adminBadge.classList.add('d-none');
    }
    
    // Ocultar a sidebar de instâncias (usuário normal tem apenas uma instância)
    const instanceSidebar = document.querySelector('.col-md-3.col-lg-2');
    if (instanceSidebar) {
        instanceSidebar.classList.add('d-none');
    }
    
    // Expandir o conteúdo principal
    const mainContent = document.querySelector('.col-md-9.col-lg-10');
    if (mainContent) {
        mainContent.classList.remove('col-md-9', 'col-lg-10');
        mainContent.classList.add('col-12');
    }
    
    // Verificar se temos um nome de usuário/instância para exibir
    const currentInstanceName = document.getElementById('currentInstanceName');
    if (currentInstanceName && authState.userName) {
        currentInstanceName.textContent = authState.userName;
    }
    
    addLog('Interface de usuário inicializada', 'info');
}

/**
 * Registrar eventos comuns para ambos os tipos de usuário
 */
function registerCommonEvents() {
    // Alternar entre QR e pareamento por telefone
    const toggleConnectionMethodBtn = document.getElementById('toggleConnectionMethodBtn');
    if (toggleConnectionMethodBtn) {
        toggleConnectionMethodBtn.addEventListener('click', toggleConnectionMethod);
    }
    
    // Registrar evento para o botão de limpar logs
    const clearLogsBtn = document.getElementById('clearLogsBtn');
    if (clearLogsBtn) {
        clearLogsBtn.addEventListener('click', () => {
            const messageLog = document.getElementById('messageLog');
            if (messageLog) {
                messageLog.innerHTML = '<div class="log-entry log-info">Logs limpos.</div>';
            }
        });
    }
    
    // Registrar evento para exportar logs
    const exportLogsBtn = document.getElementById('exportLogsBtn');
    if (exportLogsBtn) {
        exportLogsBtn.addEventListener('click', exportLogs);
    }
    
    // Registrar eventos para botões de instância
    registerInstanceEvents();
    
    // Adicionar evento para salvar estado antes de recarregar
    window.addEventListener('beforeunload', () => {
        if (instancesState.currentInstance) {
            localStorage.setItem('currentInstance', instancesState.currentInstance);
            localStorage.setItem('instanceStatus', JSON.stringify({
                connected: true,
                loggedIn: true,
                lastCheck: Date.now()
            }));
        }
    });
}

/**
 * Registrar eventos para botões de controle de instância
 */
function registerInstanceEvents() {
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
        quickLogoutBtn.addEventListener('click', () => {
            if (confirm('Tem certeza que deseja fazer logout da instância do WhatsApp?')) {
                logout();
            }
        });
    }
    
    const quickQrBtn = document.getElementById('quickQrBtn');
    if (quickQrBtn) {
        quickQrBtn.addEventListener('click', getQrCode);
    }
    
    // Refresh status
    const refreshStatus = document.getElementById('refreshStatus');
    if (refreshStatus) {
        refreshStatus.addEventListener('click', () => {
            if (instancesState && instancesState.currentInstance) {
                checkStatus(instancesState.currentInstance);
            } else {
                showToast('Selecione uma instância primeiro', 'warning');
            }
        });
    }
}

/**
 * Restaurar estado da instância após recarregar a página
 */
function restoreInstanceState() {
    // Verificar se há instâncias carregadas
    if (!instancesState.instances || instancesState.instances.length === 0) {
        return;
    }
    
    // Verificar se há dados salvos
    const savedInstanceId = localStorage.getItem('currentInstance');
    const savedStatus = localStorage.getItem('instanceStatus');
    
    if (savedInstanceId && instancesState.instances.some(i => i.id === savedInstanceId)) {
        instancesState.currentInstance = savedInstanceId;
        
        if (savedStatus) {
            try {
                const status = JSON.parse(savedStatus);
                const lastCheck = status.lastCheck || 0;
                const now = Date.now();
                
                // Se checou há menos de 5 minutos, confiar no estado salvo
                if (now - lastCheck < 300000) {
                    const instance = instancesState.instances.find(i => i.id === savedInstanceId);
                    if (instance) {
                        instance.connected = status.connected ? 1 : 0;
                        instance.isConnected = status.connected;
                        instance.loggedIn = status.loggedIn;
                        instance.isLoggedIn = status.loggedIn;
                        
                        // Atualizar também o telefone se disponível
                        if (status.phone) {
                            instance.phone = status.phone;
                        }
                        
                        // Atualizar UI para refletir o estado restaurado
                        updateStatusUI(status.connected, status.loggedIn);
                        
                        // Atualizar botões baseados no estado
                        if (typeof updateActionButtons === 'function') {
                            updateActionButtons();
                        }
                    }
                }
            } catch (error) {
                console.error('Erro ao restaurar estado da instância:', error);
            }
        }
        
        // Selecionar instância e verificar status atualizado
        if (typeof selectInstance === 'function') {
            selectInstance(savedInstanceId, false);
            
            // Verificar status com um pequeno delay para evitar múltiplas chamadas
            setTimeout(() => {
                if (typeof checkStatus === 'function') {
                    checkStatus(savedInstanceId);
                }
            }, 1000);
        }
    } else if (instancesState.instances.length > 0) {
        // Se não há instância salva ou a salva não existe mais, usar a primeira
        if (typeof selectInstance === 'function') {
            selectInstance(instancesState.instances[0].id, false);
            
            // Verificar status com delay
            setTimeout(() => {
                if (typeof checkStatus === 'function') {
                    checkStatus(instancesState.instances[0].id);
                }
            }, 1000);
        }
    }
}

/**
 * Inicializar o painel de administração
 */
function initAdminPanel() {
    // Verificar se o usuário é admin
    if (authState?.isAdmin) {
        // Mostrar o painel de administração
        const adminPanel = document.getElementById('adminDashboardPanel');
        if (adminPanel) {
            adminPanel.style.display = 'block';
            addLog('Painel de administração ativado', 'info');
            
            // Carregar dados iniciais
            loadAdminStats();
            
            // Configurar o botão de atualização
            const refreshAdminStats = document.getElementById('refreshAdminStats');
            if (refreshAdminStats) {
                refreshAdminStats.addEventListener('click', loadAdminStats);
            }
            
            // Configurar o botão de verificação do sistema
            const checkSystemBtn = document.getElementById('checkSystemBtn');
            if (checkSystemBtn) {
                checkSystemBtn.addEventListener('click', checkSystemStatus);
            }
            
            // Configurar envio de mensagem de teste
            const adminSendTestBtn = document.getElementById('adminSendTestBtn');
            if (adminSendTestBtn) {
                adminSendTestBtn.addEventListener('click', sendTestMessage);
            }
        }
    } else {
        // Esconder o painel de administração
        const adminPanel = document.getElementById('adminDashboardPanel');
        if (adminPanel) {
            adminPanel.style.display = 'none';
        }
    }
}

/**
 * Iniciar verificação periódica do status da instância atual
 */
function startStatusPolling() {
    // Limpar intervalo existente para evitar múltiplas verificações
    stopStatusPolling();
    
    // Verificar se já está em polling
    if (appState.isPolling) {
        console.log("[STATUS] Polling já está ativo, ignorando solicitação duplicada");
        return;
    }
    
    // Iniciar novo intervalo apenas se houver uma instância selecionada
    if (instancesState?.currentInstance) {
        const pollingDelay = authState?.isAdmin ? appConfig.adminReconnectDelay : 5000;
        
        console.log(`[STATUS] Iniciando polling com intervalo de ${pollingDelay}ms para instância ${instancesState.currentInstance}`);
        appState.isPolling = true;
        
        appState.statusPollingInterval = setInterval(async () => {
            try {
                // Verificar se ainda estamos autenticados
                if (!authState.isAuthenticated) {
                    console.log("[STATUS] Usuário não autenticado, parando polling");
                    stopStatusPolling();
                    return;
                }

                // Verificar se a polling ainda é relevante
                if (!instancesState?.currentInstance) {
                    console.log("[STATUS] Nenhuma instância selecionada, parando polling");
                    stopStatusPolling();
                    return;
                }
                
                // Verificar a instância atual
                console.log(`[STATUS] Verificando status da instância ${instancesState.currentInstance}`);
                const status = await checkStatusSilently(instancesState.currentInstance);
                
                // Verificar se ainda estamos autenticados após a verificação
                if (!authState.isAuthenticated) {
                    console.log("[STATUS] Autenticação perdida após verificação, parando polling");
                    stopStatusPolling();
                    return;
                }
                
                // Se a resposta foi null (erro ou fallback), não tentamos reconectar
                if (!status) return;
                
                // Se a instância retornou noSession e for admin, tentar reconectar uma vez
                if (status.needsReconnect && authState.isAdmin) {
                    if (!appConfig.isReconnecting && !appConfig.hasTriedSessionReconnect) {
                        appConfig.isReconnecting = true;
                        appConfig.hasTriedSessionReconnect = true;
                        
                        addLog('Detectado problema de sessão. Tentando reconectar...', 'warning');
                        
                        if (typeof connect === 'function') {
                            const connectResult = await connect();
                            if (connectResult) {
                                addLog('Sessão da instância reiniciada com sucesso', 'success');
                            }
                        }
                        
                        // Independente do resultado, esperar um pouco antes de continuar o polling
                        setTimeout(() => {
                            appConfig.isReconnecting = false;
                        }, 5000);
                        
                        return;
                    }
                }
                
                // Se a instância estiver desconectada e for admin, tentar reconectar
                if (status && !status.connected && authState.isAdmin && !appConfig.isReconnecting) {
                    if (appConfig.reconnectAttempts < appConfig.maxReconnectAttempts) {
                        appConfig.isReconnecting = true;
                        appConfig.reconnectAttempts++;
                        
                        addLog(`Tentativa de reconexão ${appConfig.reconnectAttempts}/${appConfig.maxReconnectAttempts}...`, 'info');
                        
                        if (typeof connect === 'function') {
                            await connect();
                        }
                        
                        // Aguardar um tempo antes de verificar novamente
                        setTimeout(async () => {
                            const newStatus = await checkStatusSilently(instancesState.currentInstance);
                            if (newStatus && newStatus.connected) {
                                appConfig.reconnectAttempts = 0;
                                appConfig.hasTriedSessionReconnect = false;
                                addLog('Reconexão bem sucedida', 'success');
                            }
                            appConfig.isReconnecting = false;
                        }, 5000);
                    } else {
                        addLog('Número máximo de tentativas de reconexão atingido', 'warning');
                        appConfig.reconnectAttempts = 0;
                        appConfig.hasTriedSessionReconnect = false;
                        // Não paramos o polling para que seja possível tentar novamente mais tarde
                    }
                } else if (status && status.connected) {
                    // Resetar contador de tentativas se estiver conectado
                    appConfig.reconnectAttempts = 0;
                    appConfig.hasTriedSessionReconnect = false;
                }
            } catch (error) {
                console.error('[STATUS] Erro no polling de status:', error);
            }
        }, pollingDelay);
        
        addLog('Verificação automática de status iniciada', 'info');
    } else {
        console.log("[STATUS] Nenhuma instância selecionada, ignorando solicitação de polling");
    }
}

/**
 * Parar verificação periódica de status
 */
function stopStatusPolling() {
    if (appState.statusPollingInterval) {
        console.log("[STATUS] Parando polling de status");
        clearInterval(appState.statusPollingInterval);
        appState.statusPollingInterval = null;
        appState.isPolling = false;
        appConfig.isReconnecting = false;
    }
    
    // Limpar também intervalos de verificação de QR
    if (instancesState && instancesState.qrCheckInterval) {
        clearInterval(instancesState.qrCheckInterval);
        instancesState.qrCheckInterval = null;
    }
}

/**
 * Verificar status silenciosamente (sem mostrar loading)
 */
async function checkStatusSilently(instanceId) {
    if (!instanceId) return null;
    
    try {
        // Usar o ID fornecido ou o ID da instância atual
        const response = await makeApiCall(`/instances/${instanceId}/status`, 'GET');
        
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
            
            // Verificar se a instância precisa de reconexão por problemas de sessão
            const needsReconnect = response.data.noSession || false;
            
            // Ignorar resposta de fallback para não atualizar a UI com dados possivelmente desatualizados
            // Mas retornar status para processamento pelo polling
            if (response.data.isOfflineFallback) {
                return {
                    connected: false,
                    loggedIn: false,
                    needsReconnect: needsReconnect
                };
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
            }
            
            // Atualizar dados na lista de instâncias
            const instance = instancesState.instances.find(i => i.id === instanceId);
            if (instance) {
                // Atualizar status
                instance.connected = isConnected ? 1 : 0;
                instance.isConnected = isConnected;
                instance.loggedIn = isLoggedIn;
                instance.isLoggedIn = isLoggedIn;
                
                // Atualizar telefone se disponível
                if (phone) {
                    instance.phone = phone;
                }
                
                // Atualizar lista visual
                if (typeof renderInstanceList === 'function') {
                    renderInstanceList();
                }
                if (typeof renderMobileInstanceList === 'function') {
                    renderMobileInstanceList();
                }
            }
            
            return {
                connected: isConnected,
                loggedIn: isLoggedIn,
                phone: phone,
                needsReconnect: needsReconnect
            };
        }
    } catch (error) {
        console.error(`Erro na verificação silenciosa de status: ${error.message}`);
    }
    
    return null;
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
    
    // Atualizar visibilidade dos botões de ação
    const actionButtons = document.querySelectorAll('.action-btn');
    
    actionButtons.forEach(button => {
        const condition = button.getAttribute('data-condition');
        
        if (!condition) return;
        
        // Definir visibilidade com base na condição
        switch (condition) {
            case 'disconnected':
                // Mostrar apenas se desconectado
                button.style.display = !isConnected ? '' : 'none';
                break;
                
            case 'connected':
                // Mostrar se conectado (com ou sem login)
                button.style.display = isConnected ? '' : 'none';
                break;
                
            case 'logged-in':
                // Mostrar apenas se conectado E logado
                button.style.display = isConnected && loggedIn ? '' : 'none';
                break;
                
            case 'waiting-qr':
                // Mostrar se conectado mas não logado (esperando QR)
                button.style.display = isConnected && !loggedIn ? '' : 'none';
                break;
        }
    });
}

/**
 * Alternar entre QR e pareamento por telefone
 */
function toggleConnectionMethod() {
    const qrSection = document.getElementById('qrCodeSection');
    const pairPhoneSection = document.getElementById('pairPhoneSection');
    
    if (!qrSection || !pairPhoneSection) return;
    
    if (qrSection.style.display === 'none') {
        // Mostrar QR, esconder pareamento por telefone
        qrSection.style.display = 'block';
        pairPhoneSection.style.display = 'none';
        addLog('Método alterado para QR Code', 'info');
    } else {
        // Mostrar pareamento por telefone, esconder QR
        qrSection.style.display = 'none';
        pairPhoneSection.style.display = 'block';
        addLog('Método alterado para pareamento por telefone', 'info');
        
        // Esconder container de código caso esteja visível
        const pairCodeContainer = document.getElementById('pairCodeContainer');
        if (pairCodeContainer) {
            pairCodeContainer.classList.add('d-none');
        }
    }
}

/**
 * Atualizar a visibilidade dos botões de ação com base no status da instância
 */
function updateActionButtons() {
    // Verificar se temos uma instância selecionada
    if (!instancesState || !instancesState.currentInstance || !instancesState.instances || instancesState.instances.length === 0) {
        return;
    }
    
    const instance = instancesState.instances.find(i => i.id == instancesState.currentInstance);
    if (!instance) return;
    
    // Status da instância
    const connected = instance.connected === true || instance.Connected === true;
    const loggedIn = instance.loggedIn === true || instance.LoggedIn === true;
    
    // Verificar todos os botões de ação
    document.querySelectorAll('.action-btn').forEach(btn => {
        const condition = btn.getAttribute('data-condition');
        
        if (!condition) return;
        
        // Analisar condição do botão
        switch (condition) {
            case 'connected':
                btn.style.display = connected ? '' : 'none';
                break;
            case 'disconnected':
                btn.style.display = !connected ? '' : 'none';
                break;
            case 'logged-in':
                btn.style.display = (connected && loggedIn) ? '' : 'none';
                break;
            case 'not-logged-in':
                btn.style.display = (connected && !loggedIn) ? '' : 'none';
                break;
            case 'waiting-qr':
                btn.style.display = (connected && !loggedIn) ? '' : 'none';
                break;
            default:
                btn.style.display = '';
                break;
        }
    });
    
    // Atualizar botões rápidos também
    const quickConnectBtn = document.getElementById('quickConnectBtn');
    const quickDisconnectBtn = document.getElementById('quickDisconnectBtn');
    const quickLogoutBtn = document.getElementById('quickLogoutBtn');
    const quickQrBtn = document.getElementById('quickQrBtn');
    
    if (quickConnectBtn) quickConnectBtn.style.display = !connected ? '' : 'none';
    if (quickDisconnectBtn) quickDisconnectBtn.style.display = connected ? '' : 'none';
    if (quickLogoutBtn) quickLogoutBtn.style.display = (connected && loggedIn) ? '' : 'none';
    if (quickQrBtn) quickQrBtn.style.display = (connected && !loggedIn) ? '' : 'none';
    
    // Garantir que todos os botões de funcionalidade estejam visíveis quando apropriado
    // Independente do tipo de usuário
    
    // Garantir que botões de webhook estejam visíveis
    const saveWebhookBtn = document.getElementById('saveWebhookBtn');
    if (saveWebhookBtn) {
        saveWebhookBtn.style.display = '';
    }
    
    // Garantir que os botões de mensagem de teste estejam visíveis quando conectado
    const sendTestMsgBtn = document.getElementById('sendTestMsgBtn');
    if (sendTestMsgBtn) {
        sendTestMsgBtn.style.display = (connected && loggedIn) ? '' : 'none';
    }
    
    // Mostrar seção de QR Code quando apropriado
    const qrCodeSection = document.getElementById('qrCodeSection');
    if (qrCodeSection) {
        qrCodeSection.style.display = (connected && !loggedIn) ? '' : 'none';
    }
    
    // Ativar botão de obter QR Code
    const getQrBtn = document.getElementById('getQrBtn');
    if (getQrBtn) {
        getQrBtn.style.display = (connected && !loggedIn) ? '' : 'none';
    }
}

// Inicialização principal da aplicação
document.addEventListener('DOMContentLoaded', async () => {
    console.log("[DEBUG] Inicializando aplicação...");
    
    // Verificar se a aplicação já foi inicializada para evitar múltiplas inicializações
    if (window.appInitialized) {
        console.log("[DEBUG] Aplicação já inicializada, ignorando solicitação duplicada");
        return;
    }
    
    // Marcar como inicializada
    window.appInitialized = true;
    
    // Sincronizar token antes de inicializar
    syncAuthToken();
    
    try {
        // Limpar o localStorage se o URL tiver o parâmetro clearCache=true
        const urlParams = new URLSearchParams(window.location.search);
        if (urlParams.has('clearCache') && urlParams.get('clearCache') === 'true') {
            localStorage.clear();
            showToast('Cache limpo com sucesso', 'success');
        }
        
        // Verificar se tem token na URL ou localStorage
        const token = await initializeToken();
        
        if (token) {
            // Verificar status de administrador
            await checkAdminStatus();
            
            // Carregar instâncias disponíveis
            if (typeof loadInstances === 'function') {
                await loadInstances();
            }
            
            // Inicializar a interface após autenticação
            initializeInterface();
        }
        
        // Verificar se é modo de depuração
        if (urlParams.has('debug') && urlParams.get('debug') === 'true') {
            appConfig.debug = true;
            console.log('Debug mode enabled');
            
            // Adicionar badge de debug ao nome da aplicação
            const navbarBrand = document.querySelector('.navbar-brand');
            if (navbarBrand) {
                navbarBrand.innerHTML += '<span class="badge bg-danger ms-2">DEBUG</span>';
            }
        }
        
        // Registrar evento para o botão de pareamento por telefone
        const pairPhoneBtn = document.getElementById('pairPhoneBtn');
        if (pairPhoneBtn) {
            pairPhoneBtn.addEventListener('click', pairPhone);
        }
        
        // Adicionar botão de logout se não existir
        const navbarNav = document.querySelector('.navbar-nav');
        if (navbarNav && !document.getElementById('logoutBtn') && authState.isAuthenticated) {
            const logoutItem = document.createElement('li');
            logoutItem.className = 'nav-item';
            logoutItem.innerHTML = `
                <a class="nav-link" href="#" id="logoutBtn">
                    <i class="bi bi-box-arrow-right"></i>Sair
                </a>
            `;
            navbarNav.appendChild(logoutItem);
            
            document.getElementById('logoutBtn').addEventListener('click', () => {
                if (confirm('Tem certeza que deseja sair?')) {
                    logout();
                }
            });
        }
    } catch (error) {
        console.error('[DEBUG] Erro na inicialização da aplicação:', error);
        showToast('Erro ao inicializar aplicação. Verifique o console.', 'danger');
    }
});

/**
 * Função para carregar estatísticas do painel admin
 */
async function loadAdminStats() {
    if (!authState?.isAdmin) return;
    
    try {
        addLog('Carregando estatísticas de administrador...', 'info');
        showLoading('Atualizando painel administrativo...');
        
        // Carregar lista de instâncias através da API
        const instancesResponse = await makeApiCall('/instances', 'GET');
        
        let instances = [];
        if (instancesResponse.success && Array.isArray(instancesResponse.data)) {
            instances = instancesResponse.data;
            
            // Contar instâncias por status
            const totalInstances = instances.length;
            let connectedCount = 0;
            let disconnectedCount = 0;
            
            // Atualizar o dropdown de instâncias
            const adminTestInstance = document.getElementById('adminTestInstance');
            if (adminTestInstance) {
                // Limpar opções existentes
                adminTestInstance.innerHTML = '<option value="">Selecione uma instância</option>';
                
                // Contar conectados/desconectados e adicionar ao dropdown
                instances.forEach(instance => {
                    const isConnected = instance.connected === 1 || instance.connected === true;
                    if (isConnected) {
                        connectedCount++;
                    } else {
                        disconnectedCount++;
                    }
                    
                    // Adicionar ao dropdown
                    const option = document.createElement('option');
                    option.value = instance.id;
                    option.textContent = `${instance.name || 'Instância ' + instance.id} (${instance.phone || 'Sem número'})`;
                    option.dataset.phone = instance.phone || '';
                    option.dataset.jid = instance.jid || '';
                    adminTestInstance.appendChild(option);
                });
                
                // Atualizar contadores
                const totalInstancesEl = document.getElementById('totalInstances');
                const connectedInstancesEl = document.getElementById('connectedInstances');
                const disconnectedInstancesEl = document.getElementById('disconnectedInstances');
                
                if (totalInstancesEl) {
                    totalInstancesEl.textContent = totalInstances;
                    totalInstancesEl.classList.add('fade-in');
                    setTimeout(() => totalInstancesEl.classList.remove('fade-in'), 500);
                }
                
                if (connectedInstancesEl) {
                    connectedInstancesEl.textContent = connectedCount;
                    connectedInstancesEl.classList.add('fade-in');
                    setTimeout(() => connectedInstancesEl.classList.remove('fade-in'), 500);
                }
                
                if (disconnectedInstancesEl) {
                    disconnectedInstancesEl.textContent = disconnectedCount;
                    disconnectedInstancesEl.classList.add('fade-in');
                    setTimeout(() => disconnectedInstancesEl.classList.remove('fade-in'), 500);
                }
            }
            
            // Preencher a tabela de instâncias
            updateAdminInstancesList(instances);
        } else {
            addLog('Falha ao carregar lista de instâncias', 'error');
        }
        
        hideLoading();
        addLog('Estatísticas de administração atualizadas', 'success');
    } catch (error) {
        hideLoading();
        addLog('Erro ao carregar estatísticas de administração', 'error');
        console.error('Erro ao carregar estatísticas:', error);
    }
}

/**
 * Enviar uma mensagem de teste
 */
async function sendTestMessage() {
    const instanceSelect = document.getElementById('adminTestInstance');
    const messageInput = document.getElementById('adminTestMessage');
    const messageTypeSelect = document.getElementById('adminTestMessageType');
    
    if (!instanceSelect || !messageInput) return;
    
    const instanceId = instanceSelect.value;
    const message = messageInput.value.trim();
    const messageType = messageTypeSelect ? messageTypeSelect.value : 'text';
    
    if (!instanceId) {
        showToast('Selecione uma instância', 'warning');
        return;
    }
    
    if (!message) {
        showToast('Digite uma mensagem', 'warning');
        return;
    }
    
    try {
        showLoading('Enviando mensagem de teste...');
        
        // Obter instância selecionada
        const selectedOption = instanceSelect.options[instanceSelect.selectedIndex];
        const phone = selectedOption.dataset.phone || '';
        const jid = selectedOption.dataset.jid || '';
        
        // Usar o número de telefone da instância se disponível, caso contrário, usar o JID
        let recipient = '';
        if (phone) {
            recipient = phone;
        } else if (jid) {
            recipient = jid.split('@')[0]; // Extrair o número do JID
        }
        
        if (!recipient) {
            hideLoading();
            showToast('Não foi possível obter o número de telefone da instância', 'error');
            return;
        }
        
        // Definir o ID da instância correta para a requisição
        window.currentInstanceForRequest = instanceId;
        
        // Payload padrão para mensagens de texto
        let payload = {
            phone: recipient
        };
        
        let endpoint = '';
        
        // Configurar o payload com base no tipo de mensagem
        switch (messageType) {
            case 'text':
                endpoint = '/chat/send/text';
                payload.message = message;
                break;
                
            case 'image':
                endpoint = '/chat/send/image';
                // Para teste simples, usamos uma imagem de exemplo
                payload.image = 'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAQAAAAEAAQMAAABmvDolAAAAA1BMVEUAAACnej3aAAAAAXRSTlMAQObYZgAAAB9JREFUaN7twQENAAAAwqD3T20PBxQAAAAAAAAAAHwYCcQAAVstBqEAAAAASUVORK5CYII=';
                payload.caption = message;
                break;
                
            case 'document':
                endpoint = '/chat/send/document';
                // Para teste, criar um documento de texto simples
                const encoder = new TextEncoder();
                const bytes = encoder.encode(message);
                const base64 = btoa(String.fromCharCode(...new Uint8Array(bytes)));
                payload.document = `data:application/octet-stream;base64,${base64}`;
                payload.fileName = 'mensagem-teste.txt';
                break;
                
            case 'location':
                endpoint = '/chat/send/location';
                payload.longitude = -46.633308; // São Paulo
                payload.latitude = -23.550520;
                payload.name = message || 'Localização de teste';
                break;
                
            default:
                endpoint = '/chat/send/text';
                payload.message = message;
        }
        
        // Enviar mensagem usando a API
        const response = await makeApiCall(endpoint, 'POST', payload);
        
        hideLoading();
        
        if (response.success) {
            showToast(`Mensagem ${messageType} enviada com sucesso`, 'success');
            addLog(`Mensagem de teste ${messageType} enviada para ${recipient}`, 'success');
            
            // Limpar campo de mensagem
            messageInput.value = '';
        } else {
            showToast(`Erro ao enviar mensagem: ${response.error || 'Falha na comunicação'}`, 'danger');
            addLog(`Falha ao enviar mensagem de teste: ${response.error || 'Erro desconhecido'}`, 'error');
        }
        
        // Limpar ID temporário da instância
        delete window.currentInstanceForRequest;
    } catch (error) {
        hideLoading();
        showToast('Erro ao enviar mensagem', 'danger');
        addLog(`Erro ao enviar mensagem: ${error.message || 'Erro desconhecido'}`, 'error');
    }
}

/**
 * Realizar o pareamento por telefone
 */
async function pairPhone() {
    if (!instancesState?.currentInstance) {
        addLog('Nenhuma instância selecionada', 'error');
        showToast('Selecione uma instância primeiro', 'warning');
        return;
    }
    
    const phoneInput = document.getElementById('phoneInput');
    if (!phoneInput) return;
    
    const phone = phoneInput.value.trim();
    if (!phone) {
        showToast('Digite um número de telefone válido', 'warning');
        return;
    }
    
    try {
        const instanceId = instancesState.currentInstance;
        showLoading(`Gerando código de pareamento para instância ${instanceId}...`);
        addLog(`Solicitando código de pareamento para ${phone} na instância ${instanceId}...`);
        
        // Verificar se a instância está conectada
        const statusResponse = await makeApiCall(`/instances/${instanceId}/status`, 'GET');
        let needsConnect = false;
        
        if (!statusResponse.success || 
            !(statusResponse.data?.Connected === true || statusResponse.data?.connected === true)) {
            needsConnect = true;
            addLog('Instância não está conectada. Iniciando conexão...', 'info');
        }
        
        // Se precisar conectar, fazemos isso antes de pedir o código
        if (needsConnect) {
            const connectResponse = await makeApiCall(`/instances/${instanceId}/connect`, 'POST', {
                Subscribe: ["Message", "ReadReceipt", "Presence", "ChatPresence"],
                Immediate: true
            });
            
            if (!connectResponse.success) {
                hideLoading();
                showToast(`Falha ao conectar para gerar código: ${connectResponse.error || 'Erro na conexão'}`, 'danger');
                addLog(`Falha ao conectar instância ${instanceId}: ${connectResponse.error || 'Erro desconhecido'}`, 'error');
                return;
            }
            
            // Aguardar um momento para que a conexão seja estabelecida
            await new Promise(resolve => setTimeout(resolve, 1000));
        }
        
        // Usar o formato correto dos parâmetros
        const response = await makeApiCall(`/instances/${instanceId}/pairphone`, 'POST', {
            Phone: phone // Primeira letra maiúscula como no backend
        });
        
        hideLoading();
        
        const pairCode = document.getElementById('pairCode');
        const pairCodeContainer = document.getElementById('pairCodeContainer');
        
        if (!pairCode || !pairCodeContainer) return;
        
        // Verificar formato da resposta
        let linkingCode;
        
        if (response.success && response.data) {
            linkingCode = response.data.LinkingCode || response.data.linkingCode;
        }
        
        if (linkingCode) {
            // Mostrar o código
            pairCode.textContent = linkingCode;
            pairCodeContainer.classList.remove('d-none');
            
            addLog(`Código de pareamento gerado com sucesso para instância ${instanceId}`, 'success');
            showToast('Código de pareamento gerado com sucesso');
            
            // Verificar status para confirmar se o pareamento foi bem-sucedido
            if (typeof startQrCheck === 'function') {
                startQrCheck(instanceId);
            }
        } else {
            const errorMsg = response.error || 'Falha ao gerar código de pareamento';
            addLog(`Falha ao gerar código para instância ${instanceId}: ${errorMsg}`, 'error');
            showToast(`Falha ao gerar código: ${errorMsg}`, 'danger');
            pairCodeContainer.classList.add('d-none');
        }
    } catch (error) {
        hideLoading();
        addLog('Erro ao gerar código de pareamento', 'error');
        showToast('Erro ao gerar código de pareamento', 'danger');
        
        const pairCodeContainer = document.getElementById('pairCodeContainer');
        if (pairCodeContainer) {
            pairCodeContainer.classList.add('d-none');
        }
    }
}

// Atualizar o encerramento para limpar intervalos
window.addEventListener('beforeunload', function() {
    // Limpar todos os intervalos e timers
    stopStatusPolling();
    
    // Salvar estado atual
    if (instancesState?.currentInstance) {
        localStorage.setItem('instanceStatus', JSON.stringify({
            connected: true,
            loggedIn: true,
            lastCheck: Date.now()
        }));
    }
}); 