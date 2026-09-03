export function parseFuncYaml(funcYaml: string): {
  name: string;
  namespace: string;
  runtime: string;
} {
  const nameMatch = funcYaml.match(/^name:\s*(.+)$/m);
  const runtimeMatch = funcYaml.match(/^runtime:\s*(.+)$/m);
  const namespaceMatch = funcYaml.match(/^namespace:\s*(.+)$/m);
  if (!runtimeMatch) throw new Error(`func.yaml missing runtime field`);
  return {
    name: nameMatch?.[1]?.trim() ?? '',
    namespace: namespaceMatch?.[1]?.trim() ?? '',
    runtime: runtimeMatch[1].trim(),
  };
}

export const handlerMap: Record<string, string> = {
  node: 'index.js',
  python: 'function/func.py',
  go: 'handle.go',
  quarkus: 'src/main/java/functions/Function.java',
};

const SYSTEM_NAMESPACE_PREFIXES = ['openshift-', 'kube-'];
const SYSTEM_NAMESPACE_NAMES = [
  'default',
  'openshift',
  'kube-system',
  'kube-public',
  'kube-node-lease',
];

export function isSystemNamespace(namespace: string): boolean {
  const name = namespace.trim();
  if (!name) return false;
  if (SYSTEM_NAMESPACE_NAMES.includes(name)) return true;
  return SYSTEM_NAMESPACE_PREFIXES.some((prefix) => name.startsWith(prefix));
}

export function isNotFoundError(err: unknown): boolean {
  if (!err || typeof err !== 'object') return false;
  const record = err as Record<string, unknown>;
  if (record.code === 404 || record.status === 404 || record.reason === 'NotFound') {
    return true;
  }
  const response = record.response as Record<string, unknown> | undefined;
  return response?.status === 404;
}

export function errorMessage(err: unknown): string {
  if (err instanceof Error) return err.message;
  if (typeof err === 'object' && err !== null && 'message' in err) {
    return String((err as { message: unknown }).message);
  }
  return String(err);
}
