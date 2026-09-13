(function initializeStatusMetrics(root) {
  function availablePercent(totalValue, availableValue) {
    const total = Number(totalValue);
    const available = Number(availableValue);
    if (!Number.isFinite(total) || !Number.isFinite(available) || total <= 0 || available < 0) return null;
    return Math.round(Math.min(100, available / total * 100) * 10) / 10;
  }

  function summarizeReceiverStatus(payload) {
    const sources = Object.values(payload && typeof payload === 'object' ? payload : {});
    if (sources.length === 0) {
      return { state: 'unconfigured', configured: 0, running: 0, failed: 0, receivedMessages: 0, lastReceivedAt: '' };
    }

    let running = 0;
    let failed = 0;
    let receivedMessages = 0;
    let lastReceivedAt = '';
    let lastReceivedTime = 0;

    for (const source of sources) {
      const hasError = typeof source?.error === 'string' && source.error.trim() !== '';
      if (source?.running === true && !hasError) running += 1;
      else failed += 1;

      const messages = Number(source?.received_messages);
      if (Number.isFinite(messages) && messages > 0) receivedMessages += messages;

      const receivedTime = Date.parse(source?.last_received_at || '');
      if (Number.isFinite(receivedTime) && receivedTime > lastReceivedTime) {
        lastReceivedTime = receivedTime;
        lastReceivedAt = source.last_received_at;
      }
    }

    const state = failed > 0 ? 'failed' : receivedMessages > 0 ? 'active' : 'waiting';
    return { state, configured: sources.length, running, failed, receivedMessages, lastReceivedAt };
  }

  const api = { availablePercent, summarizeReceiverStatus };
  root.FWLOGStatusMetrics = api;
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
})(globalThis);
