export type NamespaceRole = 'admin' | 'developer-none' | 'developer-single' | 'developer-multi';

// The single-namespace developer has no editable control, so their namespace is
// always the one namespace they can access rather than whatever was typed.
export function resolveNamespace(role: NamespaceRole, namespaces: string[], typed: string): string {
  return role === 'developer-single' ? (namespaces[0] ?? '') : typed;
}
