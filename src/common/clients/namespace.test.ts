import { resolveNamespace } from './namespace';

describe('resolveNamespace', () => {
  it('returns the sole namespace for a single-namespace developer', () => {
    expect(resolveNamespace('developer-single', ['team-a'], '')).toBe('team-a');
  });

  it('ignores the typed value for a single-namespace developer', () => {
    expect(resolveNamespace('developer-single', ['team-a'], 'typed')).toBe('team-a');
  });

  it('returns the typed value for an admin', () => {
    expect(resolveNamespace('admin', [], 'my-ns')).toBe('my-ns');
  });

  it('returns the typed value for a multi-namespace developer', () => {
    expect(resolveNamespace('developer-multi', ['a', 'b'], 'b')).toBe('b');
  });

  it('returns an empty string for a developer with no namespaces', () => {
    expect(resolveNamespace('developer-none', [], '')).toBe('');
  });
});
