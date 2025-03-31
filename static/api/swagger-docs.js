// Inicializa e configura a documentação Swagger
window.onload = function() {
  const ui = SwaggerUIBundle({
    url: "./spec.yml",
    dom_id: "#swagger-ui",
    deepLinking: true,
    presets: [
      SwaggerUIBundle.presets.apis,
      SwaggerUIStandalonePreset
    ],
    plugins: [
      SwaggerUIBundle.plugins.DownloadUrl
    ],
    layout: "StandaloneLayout",
    tagsSorter: 'alpha',
    operationsSorter: 'alpha',
    docExpansion: 'none',
    defaultModelsExpandDepth: 1,
    defaultModelExpandDepth: 1,
    filter: true,
    syntaxHighlight: {
      activate: true,
      theme: "agate"
    },
    onComplete: function() {
      // Personaliza o visual após o carregamento
      const title = document.querySelector('.topbar-wrapper a.link span');
      if (title) {
        title.innerText = 'WUZAPI Documentação';
      }
      
      // Adiciona uma mensagem informativa
      const info = document.createElement('div');
      info.className = 'info-banner';
      info.innerHTML = `
        <p>Esta documentação contém todos os endpoints disponíveis na API WUZAPI.</p>
        <p>Autenticação: Utilizar o cabeçalho <code>token</code> para usuários regulares ou <code>Authorization</code> para administradores.</p>
      `;
      info.style.padding = '10px';
      info.style.background = '#f8f8f8';
      info.style.border = '1px solid #ddd';
      info.style.borderRadius = '4px';
      info.style.margin = '10px 0';
      
      const swaggerUi = document.getElementById('swagger-ui');
      if (swaggerUi && swaggerUi.firstChild) {
        swaggerUi.insertBefore(info, swaggerUi.firstChild);
      }
    }
  });

  window.ui = ui;

  // Adiciona botão para painel de administração
  const topbar = document.querySelector('.topbar');
  if (topbar) {
    const dashboardButton = document.createElement('a');
    dashboardButton.href = '/dashboard';
    dashboardButton.className = 'dashboard-button';
    dashboardButton.textContent = 'Painel de Gerenciamento';
    dashboardButton.style.cssText = `
      position: fixed;
      top: 10px;
      right: 10px;
      z-index: 9999;
      display: inline-block;
      padding: 10px 15px;
      background-color: #25d366;
      color: white;
      font-weight: bold;
      text-decoration: none;
      border-radius: 4px;
      box-shadow: 0 2px 5px rgba(0,0,0,0.2);
      transition: background-color 0.3s;
    `;
    dashboardButton.addEventListener('mouseenter', function() {
      this.style.backgroundColor = '#128c7e';
    });
    dashboardButton.addEventListener('mouseleave', function() {
      this.style.backgroundColor = '#25d366';
    });
    
    topbar.appendChild(dashboardButton);
  }
};
