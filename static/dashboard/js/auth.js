// Authentication Module

// Estado global de autenticação
let authState = {
    userToken: '',
    isAdmin: false,
    isAuthenticated: false,
    userId: null,
    userName: '',
    tokenInitializing: false,
    lastInitialization: 0
};

/**
 * Verificar se o token é válido
 * @param {string} token - Token para verificar
 * @returns {Promise<boolean>} Resultado da validação
 */
async function validateToken(token) {
    if (!token) {
        return false;
    }
    
    try {
        // Usar o token para a requisição
        const originalToken = window.userToken;
        window.userToken = token;
        
        // Tentar fazer uma chamada à API que requer autenticação
        const headers = new Headers();
        headers.append('token', token);
        headers.append('Authorization', token);
        headers.append('Authorization', `Bearer ${token}`);
        
        const url = new URL(window.location.origin + '/api/v1/instances');
        url.searchParams.append('_', Date.now()); // Evitar cache
        
        const response = await fetch(url.toString(), {
            method: 'GET',
            headers: headers
        });
        
        // Restaurar token original
        window.userToken = originalToken;
        
        // Verificar resultado da validação
        if (response.status === 401 || response.status === 403) {
            return false;
        }
        
        if (response.ok) {
            return true;
        }
        
        return false;
    } catch (error) {
        return false;
    }
}

/**
 * Inicializar o token do usuário
 * @returns {Promise<string>} Token recuperado ou string vazia
 */
async function initializeToken() {
    // Evitar múltiplas inicializações concorrentes
    if (authState.tokenInitializing) {
        // Aguardar até a inicialização ser concluída (máximo 5 segundos)
        const initStartTime = Date.now();
        while (authState.tokenInitializing && Date.now() - initStartTime < 5000) {
            await new Promise(resolve => setTimeout(resolve, 100));
        }
        
        // Se ainda estiver inicializando após timeout, forçar continuação
        if (authState.tokenInitializing) {
            authState.tokenInitializing = false;
        } else {
            return authState.userToken || '';
        }
    }
    
    // Verificar se foi inicializado recentemente (últimos 2 segundos)
    const now = Date.now();
    if (now - authState.lastInitialization < 2000) {
        return authState.userToken || '';
    }
    
    // Marcar início da inicialização
    authState.tokenInitializing = true;
    authState.lastInitialization = now;
    
    try {
        // Sincronizar token entre todos os locais
        if (typeof syncAuthToken === 'function') {
            syncAuthToken();
        }
        
        // Ocultar elementos autenticados
        const mainContainer = document.getElementById('mainContainer');
        const authRequired = document.querySelectorAll('.auth-required');
        
        if (mainContainer) mainContainer.style.display = 'none';
        
        authRequired.forEach(el => {
            if (el.style.display) {
                el.dataset.originalDisplay = el.style.display;
            }
            el.style.display = 'none';
        });
        
        // Verificar token na URL
        const params = new URLSearchParams(window.location.search);
        let token = params.get('token');
        
        // Se não houver token na URL, tentar localStorage
        if (!token) {
            token = localStorage.getItem('wuzapiToken');
        }
        
        if (token) {
            // Validar token
            const isValid = await validateToken(token);
            
            if (isValid) {
                // Salvar token válido
                localStorage.setItem('wuzapiToken', token);
                authState.userToken = token;
                authState.isAuthenticated = true;
                window.userToken = token;
                
                // Verificar se é admin e obter informações do usuário
                await checkAdminStatus();
                
                // Inicializar estado e mostrar dashboard
                showDashboard();
                addLog('Token validado com sucesso', 'success');
                
                // Finalizar inicialização
                authState.tokenInitializing = false;
                return token;
            } else {
                // Token inválido
                localStorage.removeItem('wuzapiToken');
                authState.userToken = '';
                authState.isAuthenticated = false;
                window.userToken = null;
                
                showToast('Token inválido ou sessão expirada', 'warning');
                showLoginScreen();
                
                // Finalizar inicialização
                authState.tokenInitializing = false;
                return '';
            }
        } else {
            // Se não houver token, mostrar login
        }
        
        // Se não houver token, mostrar login
        showLoginScreen();
        
        // Finalizar inicialização
        authState.tokenInitializing = false;
        return '';
    } catch (error) {
        // Garantir que o flag seja limpo mesmo em caso de erro
        authState.tokenInitializing = false;
        return '';
    }
}

/**
 * Mostrar tela de login
 */
function showLoginScreen() {
    document.body.classList.remove('authenticated');
    
    const mainContainer = document.getElementById('mainContainer');
    const loginScreen = document.getElementById('loginScreen');
    const authRequired = document.querySelectorAll('.auth-required');
    
    if (mainContainer) mainContainer.style.display = 'none';
    if (loginScreen) loginScreen.style.display = 'flex';
    
    authRequired.forEach(el => {
        el.style.display = 'none !important';
    });
    
    // Configurar formulário de login
    const loginForm = document.getElementById('loginForm');
    if (loginForm) {
        loginForm.addEventListener('submit', async function(e) {
            e.preventDefault();
            const token = document.getElementById('loginToken').value.trim();
            if (!token) {
                showToast('Digite um token válido', 'warning');
                return;
            }
            
            showLoading('Validando token...');
            const isValid = await validateToken(token);
            hideLoading();
            
            if (isValid) {
                saveToken(token);
                await checkAdminStatus();
                showDashboard();
                if (typeof loadInstances === 'function') {
                    await loadInstances();
                }
            } else {
                showToast('Token inválido', 'danger');
            }
        });
    }
    
    // Configurar login como admin
    const loginAsAdminBtn = document.getElementById('loginAsAdminBtn');
    if (loginAsAdminBtn) {
        loginAsAdminBtn.addEventListener('click', loginAsAdmin);
    }
}

/**
 * Função para login como administrador
 */
async function loginAsAdmin() {
    try {
        showLoading('Autenticando como administrador...');
        
        // Tentar obter o token admin
        const adminPassword = prompt("Digite a senha de administrador:");
        if (!adminPassword) {
            hideLoading();
            return false;
        }
        
        // Primeiro tenta obter token do endpoint /admin/token
        const response = await fetch(`/api/v1/admin/token?password=${encodeURIComponent(adminPassword)}`, {
            method: 'GET'
        });
        
        let adminToken = '';
        
        if (response.ok) {
            const data = await response.json();
            if (data.success && data.token) {
                adminToken = data.token;
            }
        }
        
        // Se não conseguiu obter token, tenta com "admin" (compatibilidade)
        if (!adminToken) {
            adminToken = "admin";
        }
        
        // Tentar validar o token admin
        const isValid = await validateToken(adminToken);
        
        if (isValid) {
            saveToken(adminToken);
            await checkAdminStatus();
            showDashboard();
            if (typeof loadInstances === 'function') {
                await loadInstances();
            }
            hideLoading();
            return true;
        } else {
            hideLoading();
            showToast('Acesso de administrador negado', 'danger');
            return false;
        }
    } catch (error) {
        console.error('Erro ao fazer login como admin:', error);
        hideLoading();
        showToast('Erro ao autenticar como administrador', 'danger');
        return false;
    }
}

/**
 * Mostrar dashboard
 */
function showDashboard() {
    const loginScreen = document.getElementById('loginScreen');
    const mainContainer = document.getElementById('mainContainer');
    const authRequired = document.querySelectorAll('.auth-required');
    
    if (loginScreen) loginScreen.style.display = 'none';
    if (mainContainer) mainContainer.style.display = 'block';
    
    authRequired.forEach(el => {
        // Restaurar display original ou usar '' (vazio) como padrão
        const originalDisplay = el.dataset.originalDisplay || '';
        el.style.display = originalDisplay;
    });
    
    document.body.classList.add('authenticated');
    
    // Mostrar elementos específicos com base no tipo de usuário
    if (authState.isAdmin) {
        document.querySelectorAll('.admin-only').forEach(el => {
            el.style.display = '';
        });
    } else {
        document.querySelectorAll('.admin-only').forEach(el => {
            el.style.display = 'none';
        });
    }
    
    // Elementos de usuário sempre visíveis para permitir gerenciamento da instância
    document.querySelectorAll('.user-only').forEach(el => {
        el.style.display = '';
    });
    
    // Inicializar interface
    if (typeof initializeInterface === 'function') {
        initializeInterface();
    }
}

/**
 * Salvar o token do usuário
 * @param {string} token - Token para salvar
 * @returns {boolean} Sucesso da operação
 */
function saveToken(token) {
    if (!token) {
        showToast('Por favor, informe um token válido', 'warning');
        return false;
    }
    
    authState.userToken = token;
    authState.isAuthenticated = true;
    window.userToken = token;
    localStorage.setItem('wuzapiToken', token);
    
    // Atualizar URL com o token
    const currentUrl = new URL(window.location.href);
    currentUrl.searchParams.set('token', token);
    window.history.replaceState({}, '', currentUrl.toString());
    
    addLog('Token salvo com sucesso', 'success');
    return true;
}

/**
 * Verificar o status de administrador e obter informações do usuário
 * @returns {Promise<boolean>} Status de administrador
 */
async function checkAdminStatus() {
    try {
        // Para garantir que userToken está disponível globalmente
        if (typeof syncAuthToken === 'function') {
            syncAuthToken();
        }
        
        // Tentar explicitamente com header 'Authorization' sem 'Bearer'
        const headers = new Headers();
        const token = window.userToken || authState.userToken || localStorage.getItem('wuzapiToken');
        
        if (!token) {
            authState.isAdmin = false;
            return false;
        }
        
        // Verificar se é admin
        headers.append('Authorization', token);
        
        const url = new URL(window.location.origin + '/api/v1/admin/status');
        url.searchParams.append('_', Date.now()); // Evitar cache
        
        const response = await fetch(url.toString(), {
            method: 'GET',
            headers: headers
        });
        
        // Verificar se é admin com base na resposta
        if (response.status === 401 || response.status === 403) {
            authState.isAdmin = false;
            await getUserInfo();
            applyUserInterfaceRestrictions();
            return false;
        }
        
        if (response.ok) {
            try {
                const data = await response.json();
                authState.isAdmin = data.success && data.data && data.data.isAdmin === true;
                
                if (authState.isAdmin) {
                    // Mostrar badge de admin
                    const adminBadge = document.getElementById('adminBadge');
                    if (adminBadge) adminBadge.classList.remove('d-none');
                    
                    // Preencher ID do usuário admin
                    authState.userId = data.data.userId || 1;
                    authState.userName = data.data.userName || 'Administrador';
                    
                    addLog('Conectado como administrador', 'success');
                    showToast('Acesso de administrador concedido', 'success');
                } else {
                    // Esconder badge de admin
                    const adminBadge = document.getElementById('adminBadge');
                    if (adminBadge) adminBadge.classList.add('d-none');
                    
                    // Obter informações do usuário normal
                    await getUserInfo();
                }
                
                // Aplicar restrições de interface
                applyUserInterfaceRestrictions();
                
                return authState.isAdmin;
            } catch (error) {
                authState.isAdmin = false;
                await getUserInfo();
                return false;
            }
        }
        
        authState.isAdmin = false;
        await getUserInfo();
        return false;
    } catch (error) {
        authState.isAdmin = false;
        await getUserInfo();
        return false;
    }
}

/**
 * Obter informações do usuário atual
 */
async function getUserInfo() {
    try {
        // Chamar API para obter detalhes do usuário
        const token = window.userToken || authState.userToken || localStorage.getItem('wuzapiToken');
        
        if (!token) {
            return;
        }
        
        // Usar múltiplos formatos de autenticação
        const headers = new Headers();
        headers.append('token', token);
        headers.append('Authorization', token);
        headers.append('Authorization', `Bearer ${token}`);
        
        const url = new URL(window.location.origin + '/api/v1/instances');
        url.searchParams.append('_', Date.now()); // Evitar cache
        
        const response = await fetch(url.toString(), {
            method: 'GET',
            headers: headers
        });
        
        if (response.ok) {
            try {
                const data = await response.json();
                if (data.success && data.data && Array.isArray(data.data) && data.data.length > 0) {
                    // Para usuário normal, pegar a primeira instância disponível
                    const instance = data.data[0];
                    
                    // Atualizar informações do usuário
                    authState.userId = instance.id;
                    authState.userName = instance.name || `Instância ${instance.id}`;
                    
                    // Atualizar display do nome do usuário
                    const userNameDisplay = document.getElementById('userNameDisplay');
                    if (userNameDisplay) {
                        userNameDisplay.textContent = authState.userName || 'Usuário';
                    }
                }
            } catch (error) {
                // Erro ao processar informações
            }
        }
    } catch (error) {
        // Erro ao obter informações
    }
}

/**
 * Obter o token de autenticação atual
 * @returns {string} Token atual
 */
function getAuthToken() {
    return authState.userToken || '';
}

/**
 * Verifica se há um token salvo e exibe o modal de token se necessário
 */
function checkAuthentication() {
    if (!authState.userToken) {
        // Mostrar modal de token automaticamente
        const tokenModal = document.getElementById('tokenModal');
        if (tokenModal) {
            new bootstrap.Modal(tokenModal).show();
        }
        return false;
    }
    return true;
}

/**
 * Obtém o status atual da autenticação
 * @returns {object} Estado de autenticação
 */
function getAuthState() {
    return {
        userToken: authState.userToken,
        isAdmin: authState.isAdmin,
        isAuthenticated: authState.isAuthenticated,
        userId: authState.userId,
        userName: authState.userName
    };
}

/**
 * Fazer logout
 */
function logout() {
    localStorage.removeItem('wuzapiToken');
    authState.userToken = '';
    authState.isAdmin = false;
    authState.isAuthenticated = false;
    authState.userId = null;
    authState.userName = '';
    window.userToken = null;
    
    // Remover classe de autenticado
    document.body.classList.remove('authenticated');
    
    // Limpar estado das instâncias
    if (window.instancesState) {
        window.instancesState = {
            instances: [],
            currentInstance: null,
            qrCheckInterval: null
        };
    }
    
    // Parar polling e limpar intervalos
    if (typeof stopStatusPolling === 'function') {
        stopStatusPolling();
    }
    
    // Remover token da URL
    const currentUrl = new URL(window.location.href);
    currentUrl.searchParams.delete('token');
    window.history.replaceState({}, '', currentUrl.toString());
    
    showLoginScreen();
    addLog('Logout realizado com sucesso', 'info');
    showToast('Logout realizado com sucesso', 'success');
}

/**
 * Aplicar restrições de interface baseadas no status de administrador do usuário
 */
function applyUserInterfaceRestrictions() {
    const isAdmin = authState?.isAdmin === true;
    
    // Mostrar/ocultar opções de gerenciamento de instâncias para admin
    const manageInstancesLink = document.querySelector('.nav-link[data-bs-toggle="modal"][data-bs-target="#manageInstancesModal"]');
    if (manageInstancesLink) {
        manageInstancesLink.style.display = isAdmin ? '' : 'none';
    }
    
    // Mostrar/ocultar botões de adicionar instância para admin
    const addInstanceButtons = document.querySelectorAll('[data-bs-target="#addInstanceModal"]');
    if (addInstanceButtons) {
        addInstanceButtons.forEach(btn => {
            btn.style.display = isAdmin ? '' : 'none';
        });
    }
    
    // Substituir botão de verificar sistema por verificar instância para não-admins
    const checkSystemStatusBtn = document.getElementById('checkSystemStatusBtn');
    if (checkSystemStatusBtn) {
        if (isAdmin) {
            checkSystemStatusBtn.innerHTML = '<i class="bi bi-check-circle"></i>Verificar Sistema';
            checkSystemStatusBtn.setAttribute('data-function', 'check-system-status');
            
            // Remover evento antigo se existir
            checkSystemStatusBtn.removeEventListener('click', checkInstanceStatus);
            // Adicionar evento para admin
            checkSystemStatusBtn.addEventListener('click', checkSystemStatus);
        } else {
            checkSystemStatusBtn.innerHTML = '<i class="bi bi-check-circle"></i>Verificar Status';
            checkSystemStatusBtn.setAttribute('data-function', 'check-instance-status');
            
            // Remover evento antigo se existir
            checkSystemStatusBtn.removeEventListener('click', checkSystemStatus);
            // Adicionar evento para usuário normal
            checkSystemStatusBtn.addEventListener('click', checkInstanceStatus);
        }
    }
    
    // Mostrar elementos específicos para admin/usuário
    // Admin vê tudo, usuário normal vê apenas o necessário
    document.querySelectorAll('.admin-only').forEach(el => {
        el.style.display = isAdmin ? '' : 'none';
    });
    
    // Todos podem ver elementos de usuário para gerenciar instâncias
    document.querySelectorAll('.user-only').forEach(el => {
        el.style.display = '';
    });
    
    // Para usuários não-admin, ocultar a coluna lateral de instâncias e expandir o conteúdo principal
    if (!isAdmin) {
        const sidebarCol = document.querySelector('.col-md-3.col-lg-2');
        if (sidebarCol) {
            sidebarCol.classList.add('d-none');
        }
        
        const mainCol = document.querySelector('.col-md-9.col-lg-10');
        if (mainCol) {
            mainCol.className = 'col-12';
        }
        
        // Garantir que o dashboard de instância esteja visível
        const instanceDashboard = document.getElementById('instanceDashboard');
        if (instanceDashboard) {
            instanceDashboard.style.display = '';
        }
    }
    
    // Atualizar badge de admin no header
    const adminBadge = document.getElementById('adminBadge');
    if (adminBadge) {
        adminBadge.classList.toggle('d-none', !isAdmin);
    }
    
    // Atualizar o nome de usuário/instância no cabeçalho
    const userNameDisplay = document.getElementById('userNameDisplay');
    if (userNameDisplay) {
        userNameDisplay.textContent = authState.userName || 'Usuário';
    }
}

/**
 * Tratar erro de autenticação
 */
window.handleAuthError = function() {
    // Tentar sincronizar token
    if (typeof syncAuthToken === 'function') {
        const result = syncAuthToken();
        if (result) {
            return;
        }
    }
    
    // Se não conseguir sincronizar, limpar token e redirecionar para login
    localStorage.removeItem('wuzapiToken');
    authState.userToken = '';
    authState.isAuthenticated = false;
    window.userToken = null;
    
    // Mostrar mensagem e redirecionar após timeout
    showToast('Sessão expirada ou token inválido', 'warning');
    setTimeout(() => {
        window.location.href = '/dashboard?clearCache=true';
    }, 2000);
}

// Inicialização do módulo de autenticação
document.addEventListener('DOMContentLoaded', () => {
    // Verificar se a autenticação já foi inicializada
    if (window.authInitialized) {
        return;
    }
    
    // Marcar como inicializado
    window.authInitialized = true;
    
    // Inicializar token
    initializeToken();
}); 