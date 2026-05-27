function addItem() {
    const container = document.getElementById('itemsContainer');
    if (!container) return;

    const row = document.createElement('div');
    row.className = 'invoice-item-row';
    row.innerHTML = `
        <input type="text" name="item_description[]" placeholder="描述" required>
        <input type="number" name="item_quantity[]" placeholder="数量" step="0.01" value="1" required>
        <input type="number" name="item_unit_price[]" placeholder="单价" step="0.01" value="0" required>
        <button type="button" class="btn btn-danger btn-sm remove-item" onclick="removeItem(this)">删除</button>
    `;
    container.appendChild(row);
    updateSummary();
}

function removeItem(btn) {
    const container = document.getElementById('itemsContainer');
    if (container && container.children.length > 1) {
        btn.parentElement.remove();
        updateSummary();
    }
}

function updateSummary() {
    const rows = document.querySelectorAll('.invoice-item-row, .quote-item-row');
    let subtotal = 0;

    rows.forEach(row => {
        const qty = parseFloat(row.querySelector('input[name*="quantity"]').value) || 0;
        const price = parseFloat(row.querySelector('input[name*="unit_price"]').value) || 0;
        subtotal += qty * price;
    });

    const taxRate = parseFloat(document.getElementById('taxRate')?.value || 0);
    const tax = subtotal * taxRate / 100;
    const total = subtotal + tax;

    const subtotalDisplay = document.getElementById('subtotalDisplay');
    const taxDisplay = document.getElementById('taxDisplay');
    const totalDisplay = document.getElementById('totalDisplay');

    if (subtotalDisplay) subtotalDisplay.textContent = '¥' + subtotal.toFixed(2);
    if (taxDisplay) taxDisplay.textContent = '¥' + tax.toFixed(2);
    if (totalDisplay) totalDisplay.textContent = '¥' + total.toFixed(2);
}

document.addEventListener('input', function(e) {
    if (e.target.matches('input[name*="quantity"], input[name*="unit_price"], #taxRate')) {
        updateSummary();
    }
});

function updateTimer() {
    const timer = document.getElementById('liveTimer');
    if (!timer) return;

    const startTime = new Date(timer.dataset.start);
    const now = new Date();
    const diff = Math.floor((now - startTime) / 1000);

    const hours = Math.floor(diff / 3600);
    const minutes = Math.floor((diff % 3600) / 60);
    const seconds = diff % 60;

    timer.textContent =
        String(hours).padStart(2, '0') + ':' +
        String(minutes).padStart(2, '0') + ':' +
        String(seconds).padStart(2, '0');
}

setInterval(updateTimer, 1000);
updateTimer();

const clientSelect = document.getElementById('clientSelect');
const projectSelect = document.getElementById('projectSelect');

if (clientSelect && projectSelect) {
    clientSelect.addEventListener('change', function() {
        const clientId = this.value;
        const options = projectSelect.querySelectorAll('option');

        options.forEach(option => {
            if (option.value === '') {
                option.style.display = '';
            } else if (clientId === '') {
                option.style.display = '';
            } else {
                option.style.display = option.dataset.client === clientId ? '' : 'none';
            }
        });

        projectSelect.value = '';
    });
}

document.addEventListener('DOMContentLoaded', function() {
    updateSummary();

    const dateInputs = document.querySelectorAll('input[type="date"]');
    const today = new Date().toISOString().split('T')[0];
    dateInputs.forEach(input => {
        if (!input.value) {
            input.value = today;
        }
    });
});
