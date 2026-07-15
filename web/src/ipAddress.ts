import ipaddr from 'ipaddr.js';

export type CidrAliasSetting = {
  cidr?: string;
  alias?: string;
  enabled?: boolean;
};

function parseAddress(value?: string) {
  const text = String(value || '').trim();
  if (!text) return null;
  try {
    return ipaddr.process(text);
  } catch {
    return null;
  }
}

export function formatIPPort(ip?: string, port?: number) {
  if (!ip) return '-';
  if (!port) return ip;
  const parsed = parseAddress(ip);
  return parsed?.kind() === 'ipv6' ? `[${ip}]:${port}` : `${ip}:${port}`;
}

export function matchCidrAlias(ip: string | undefined, aliases: CidrAliasSetting[]) {
  const address = parseAddress(ip);
  if (!address) return '';

  let best = '';
  let bestPrefix = -1;
  aliases.forEach((item) => {
    if (item.enabled === false || !item.cidr || !item.alias) return;
    try {
      const [range, prefix] = ipaddr.parseCIDR(item.cidr.trim());
      if (address.kind() !== range.kind() || prefix <= bestPrefix) return;
      if (address.match(range, prefix)) {
        best = item.alias;
        bestPrefix = prefix;
      }
    } catch {
      // Invalid saved aliases are ignored so one bad entry cannot break the log table.
    }
  });
  return best;
}

export function geoFallback(ip?: string) {
  const address = parseAddress(ip);
  if (!address) return '-';

  const localRanges = new Set([
    'private',
    'loopback',
    'linkLocal',
    'uniqueLocal',
    'carrierGradeNat',
    'unspecified',
  ]);
  return localRanges.has(address.range()) ? '内网' : '未知公网';
}
