/**
 * Esconde o modal de carregamento
 */
function hideLoading() {
    const loadingModal = document.getElementById('loadingModal');
    if (loadingModal) {
        const modal = bootstrap.Modal.getInstance(loadingModal);
        if (modal) {
            modal.hide();
            
            // Remover o modal após ocultar para manter o DOM limpo
            loadingModal.addEventListener('hidden.bs.modal', () => {
                setTimeout(() => {
                    if (loadingModal.parentNode) {
                        loadingModal.parentNode.removeChild(loadingModal);
                    }
                }, 300);
            }, { once: true });
        }
    }
} 