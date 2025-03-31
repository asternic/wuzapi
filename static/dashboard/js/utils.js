// Funções utilitárias para o WuzAPI Dashboard

/**
 * Adiciona uma mensagem de log ao container de logs
 * @param {string} message - Mensagem a ser registrada
 * @param {string} type - Tipo de log (info, success, error, warning)
 */
function addLog(message, type = 'info') {
    const messageLog = document.getElementById('messageLog');
    if (!messageLog) return;
    
    const timestamp = new Date().toLocaleTimeString();
    const logEntry = document.createElement('div');
    logEntry.className = `log-entry log-${type}`;
    logEntry.innerHTML = `<span class="log-time">[${timestamp}]</span> ${message}`;
    
    // Adicionar ao topo do log
    messageLog.insertBefore(logEntry, messageLog.firstChild);
    
    // Limitar o número de entradas de log
    const maxLogEntries = 100;
    const entries = messageLog.querySelectorAll('.log-entry');
    if (entries.length > maxLogEntries) {
        for (let i = maxLogEntries; i < entries.length; i++) {
            entries[i].remove();
        }
    }
    
    // Também exibir no console para debug
    const consoleMethod = type === 'error' ? 'error' : 
                          type === 'warning' ? 'warn' : 
                          type === 'success' ? 'info' : 'log';
    console[consoleMethod](`[WuzAPI] ${message}`);
}

/**
 * Exibe um modal de carregamento com efeito de luxo
 * @param {string} message - Mensagem a ser exibida no modal
 */
function showLoading(message = 'Processando sua solicitação...') {
    // Verificar se já existe um modal de carregamento
    let loadingModal = document.getElementById('loadingModal');
    
    // Se já existe um modal aberto, apenas atualizar a mensagem
    if (loadingModal && loadingModal.classList.contains('show')) {
        const loadingMessage = document.getElementById('loadingMessage');
        if (loadingMessage) {
            loadingMessage.textContent = message;
        }
        return;
    }
    
    if (!loadingModal) {
        // Criar o modal dinamicamente
        const modalHtml = `
            <div class="modal fade" id="loadingModal" tabindex="-1" data-bs-backdrop="static" aria-hidden="true">
                <div class="modal-dialog modal-dialog-centered">
                    <div class="modal-content glass-card border-0">
                        <div class="modal-body text-center p-5">
                            <div class="loading-spinner mb-4"></div>
                            <h5 id="loadingMessage" class="fw-bold">${message}</h5>
                        </div>
                    </div>
                </div>
            </div>
            <style>
                .loading-spinner {
                    display: inline-block;
                    width: 3.5rem;
                    height: 3.5rem;
                    border-radius: 50%;
                    border: 3px solid rgba(0, 0, 0, 0.1);
                    border-top-color: var(--primary-color);
                    animation: spinner 1s ease-in-out infinite;
                    position: relative;
                }
                
                .loading-spinner::after {
                    content: '';
                    position: absolute;
                    top: -3px;
                    left: -3px;
                    right: -3px;
                    bottom: -3px;
                    border-radius: 50%;
                    border: 3px solid transparent;
                    border-top-color: var(--secondary-color);
                    opacity: 0.6;
                    animation: spinner 1.5s linear infinite;
                }
                
                @keyframes spinner {
                    0% { transform: rotate(0deg); }
                    100% { transform: rotate(360deg); }
                }
            </style>
        `;
        
        // Adicionar ao body
        const div = document.createElement('div');
        div.innerHTML = modalHtml;
        document.body.appendChild(div.firstElementChild);
        loadingModal = document.getElementById('loadingModal');
    } else {
        // Atualizar mensagem
        const loadingMessage = document.getElementById('loadingMessage');
        if (loadingMessage) {
            loadingMessage.textContent = message;
        }
    }
    
    try {
        // Inicializar e mostrar
        if (window.bootstrap) {
            const modal = new bootstrap.Modal(loadingModal);
            modal.show();
        } else {
            // Fallback para quando bootstrap não está disponível
            loadingModal.classList.add('show');
            loadingModal.style.display = 'block';
            document.body.classList.add('modal-open');
            
            // Adicionar backdrop
            const backdrop = document.createElement('div');
            backdrop.className = 'modal-backdrop fade show';
            document.body.appendChild(backdrop);
        }
    } catch (error) {
        console.error('Erro ao mostrar modal de carregamento:', error);
    }
}

/**
 * Exibe uma notificação toast de visual premium
 * @param {string} message - Mensagem a ser exibida
 * @param {string} type - Tipo de notificação (success, warning, danger, primary)
 */
function showToast(message, type = 'success', duration = 3000) {
    // Criar container de toasts se não existir
    let toastContainer = document.querySelector('.toast-container');
    if (!toastContainer) {
        toastContainer = document.createElement('div');
        toastContainer.className = 'toast-container';
        document.body.appendChild(toastContainer);
    }
    
    // Criar o elemento toast
    const toastElement = document.createElement('div');
    toastElement.className = `toast align-items-center border-0 show`;
    toastElement.style.backgroundColor = 'white';
    
    // Definir a cor da borda baseada no tipo
    let borderColor = '';
    switch (type) {
        case 'success':
            borderColor = 'var(--success-color)';
            break;
        case 'danger':
            borderColor = 'var(--error-color)';
            break;
        case 'warning':
            borderColor = 'var(--warning-color)';
            break;
        case 'info':
            borderColor = 'var(--info-color)';
            break;
        case 'primary':
            borderColor = 'var(--primary-color)';
            break;
        default:
            borderColor = 'var(--primary-color)';
    }
    
    // Aplicar borda colorida
    toastElement.style.borderLeft = `4px solid ${borderColor}`;
    
    // Conteúdo do toast
    toastElement.innerHTML = `
        <div class="d-flex">
            <div class="toast-body">
                ${message}
            </div>
            <button type="button" class="btn-close me-2 m-auto" data-bs-dismiss="toast"></button>
        </div>
    `;
    
    // Adicionar ao container
    toastContainer.appendChild(toastElement);
    
    // Adicionar evento para fechar o toast
    const closeBtn = toastElement.querySelector('.btn-close');
    if (closeBtn) {
        closeBtn.addEventListener('click', () => {
            toastElement.remove();
        });
    }
    
    // Remover o toast após a duração
    setTimeout(() => {
        // Animação de fade out
        toastElement.style.transition = 'opacity 0.5s ease';
        toastElement.style.opacity = '0';
        
        // Remover o elemento após animação
        setTimeout(() => {
            toastElement.remove();
        }, 500);
    }, duration);
}

/**
 * Cria um container para as notificações toast
 * @returns {HTMLElement} Container para toasts
 */
function createToastContainer() {
    const container = document.createElement('div');
    container.className = 'toast-container position-fixed top-0 end-0 p-3';
    container.style.zIndex = '1070';
    document.body.appendChild(container);
    return container;
}

/**
 * Atualiza os indicadores de status na interface
 * @param {boolean} connected - Status de conexão
 * @param {boolean} loggedIn - Status de autenticação
 */
function updateStatusUI(connected, loggedIn) {
    const statusIndicator = document.getElementById('statusIndicator');
    const quickStatusIndicator = document.getElementById('quickStatusIndicator');
    const statusText = document.getElementById('statusText');
    const instanceStatus = document.getElementById('instanceStatus');
    
    if (!statusIndicator || !statusText || !instanceStatus) return;
    
    if (connected && loggedIn) {
        statusIndicator.className = 'status-indicator status-connected';
        if (quickStatusIndicator) quickStatusIndicator.className = 'status-indicator status-connected';
        statusText.textContent = 'Conectado';
        if (document.getElementById('statusTextMobile')) {
            document.getElementById('statusTextMobile').textContent = 'Conectado';
        }
        instanceStatus.textContent = 'Conectado e Autenticado';
        instanceStatus.className = 'badge bg-success';
    } else if (connected) {
        statusIndicator.className = 'status-indicator status-waiting';
        if (quickStatusIndicator) quickStatusIndicator.className = 'status-indicator status-waiting';
        statusText.textContent = 'Aguardando QR';
        if (document.getElementById('statusTextMobile')) {
            document.getElementById('statusTextMobile').textContent = 'Aguardando QR';
        }
        instanceStatus.textContent = 'Aguardando Autenticação';
        instanceStatus.className = 'badge bg-warning';
    } else {
        statusIndicator.className = 'status-indicator status-disconnected';
        if (quickStatusIndicator) quickStatusIndicator.className = 'status-indicator status-disconnected';
        statusText.textContent = 'Desconectado';
        if (document.getElementById('statusTextMobile')) {
            document.getElementById('statusTextMobile').textContent = 'Desconectado';
        }
        instanceStatus.textContent = 'Desconectado';
        instanceStatus.className = 'badge bg-danger';
    }
}

/**
 * Converte um arquivo para Base64
 * @param {File} file - O arquivo a ser convertido
 * @returns {Promise<string>} String Base64 do arquivo
 */
function fileToBase64(file) {
    return new Promise((resolve, reject) => {
        const reader = new FileReader();
        reader.readAsDataURL(file);
        reader.onload = () => {
            // Remover prefixo data:image/jpeg;base64,
            const base64 = reader.result.split(',')[1];
            resolve(base64);
        };
        reader.onerror = error => reject(error);
    });
}

/**
 * Exporta os logs como arquivo de texto
 */
function exportLogs() {
    const messageLog = document.getElementById('messageLog');
    
    if (!messageLog || messageLog.children.length === 0) {
        showToast('Não há logs para exportar', 'warning');
        return;
    }
    
    try {
        // Coletar todos os logs
        let logsText = "=== WuzAPI - Logs de Atividade ===\n";
        logsText += `Data: ${new Date().toLocaleString()}\n`;
        const currentInstanceName = document.getElementById('currentInstanceName');
        if (currentInstanceName) {
            logsText += `Instância: ${currentInstanceName.textContent}\n`;
        }
        logsText += "===================================\n\n";
        
        // Adicionar cada entrada de log
        Array.from(messageLog.children).forEach(entry => {
            logsText += entry.textContent + "\n";
        });
        
        // Criar blob e link para download
        const blob = new Blob([logsText], { type: 'text/plain' });
        const url = URL.createObjectURL(blob);
        
        const a = document.createElement('a');
        a.href = url;
        a.download = `wuzapi-logs-${new Date().toISOString().split('T')[0]}.txt`;
        document.body.appendChild(a);
        a.click();
        
        // Limpar
        setTimeout(() => {
            document.body.removeChild(a);
            URL.revokeObjectURL(url);
        }, 100);
        
        addLog('Logs exportados com sucesso', 'success');
        showToast('Logs exportados com sucesso');
    } catch (error) {
        addLog('Erro ao exportar logs', 'error');
        showToast('Erro ao exportar logs', 'danger');
    }
}

/**
 * Esconde o modal de carregamento
 */
function hideLoading() {
    const loadingModal = document.getElementById('loadingModal');
    if (!loadingModal) return;
    
    try {
        if (window.bootstrap) {
            const modal = bootstrap.Modal.getInstance(loadingModal);
            if (modal) {
                modal.hide();
                
                // Remover o modal após ocultar para manter o DOM limpo
                loadingModal.addEventListener('hidden.bs.modal', () => {
                    setTimeout(() => {
                        if (loadingModal.parentNode) {
                            loadingModal.parentNode.removeChild(loadingModal);
                        }
                        
                        // Remover backdrops que possam ter ficado
                        document.querySelectorAll('.modal-backdrop').forEach(backdrop => {
                            backdrop.remove();
                        });
                    }, 300);
                }, { once: true });
            }
        } else {
            // Fallback para quando bootstrap não está disponível
            loadingModal.classList.remove('show');
            loadingModal.style.display = 'none';
            document.body.classList.remove('modal-open');
            
            // Remover backdrop
            document.querySelectorAll('.modal-backdrop').forEach(backdrop => {
                backdrop.remove();
            });
            
            // Remover o próprio modal
            setTimeout(() => {
                if (loadingModal.parentNode) {
                    loadingModal.parentNode.removeChild(loadingModal);
                }
            }, 300);
        }
    } catch (error) {
        console.error('Erro ao esconder modal de carregamento:', error);
        
        // Remover manualmente em caso de erro
        if (loadingModal.parentNode) {
            loadingModal.parentNode.removeChild(loadingModal);
        }
        
        // Remover backdrops
        document.querySelectorAll('.modal-backdrop').forEach(backdrop => {
            backdrop.remove();
        });
        
        // Restaurar body
        document.body.classList.remove('modal-open');
    }
}

/**
 * Gerar um ID aleatório
 * @param {number} length - Comprimento do ID (padrão: 10)
 * @returns {string} ID gerado
 */
function generateRandomId(length = 10) {
    const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789';
    let id = '';
    
    for (let i = 0; i < length; i++) {
        id += chars.charAt(Math.floor(Math.random() * chars.length));
    }
    
    return id;
}

// Exportar funções para uso global
window.showToast = showToast;
window.addLog = addLog;
window.showLoading = showLoading;
window.hideLoading = hideLoading;
window.exportLogs = exportLogs;
window.generateRandomId = generateRandomId; 