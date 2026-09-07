import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { NamespaceField } from './NamespaceField';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

describe('NamespaceField', () => {
  const onChange = vi.fn();

  const defaultProps = {
    canCreateNamespaces: true,
    namespaces: [] as string[],
    loading: false,
    value: '',
    onChange,
  };

  afterEach(() => {
    vi.clearAllMocks();
  });

  describe('user who can create namespaces', () => {
    it('renders a free-text namespace input', () => {
      render(<NamespaceField {...defaultProps} />);

      expect(screen.getByRole('textbox', { name: /Namespace/ })).toBeInTheDocument();
    });

    it('calls onChange as the user types', async () => {
      const user = userEvent.setup();
      render(<NamespaceField {...defaultProps} />);

      await user.type(screen.getByRole('textbox', { name: /Namespace/ }), 'x');

      expect(onChange).toHaveBeenCalledWith('x');
    });

    it('shows a system namespace warning for a system namespace', () => {
      render(<NamespaceField {...defaultProps} value="openshift-monitoring" />);

      expect(screen.getByText(/system namespace/i)).toBeInTheDocument();
    });
  });

  describe('developer with no namespaces', () => {
    const noneProps = {
      ...defaultProps,
      canCreateNamespaces: false,
      namespaces: [] as string[],
    };

    it('shows a generic no-namespaces message and no input', () => {
      render(<NamespaceField {...noneProps} />);

      expect(screen.getByText(/no namespaces available/i)).toBeInTheDocument();
      expect(screen.queryByRole('textbox', { name: /Namespace/ })).not.toBeInTheDocument();
      expect(screen.queryByRole('combobox', { name: /Namespace/ })).not.toBeInTheDocument();
    });
  });

  describe('developer with exactly one namespace', () => {
    const singleProps = {
      ...defaultProps,
      canCreateNamespaces: false,
      namespaces: ['team-a'],
      value: '',
    };

    it('shows the sole namespace even when no value is set, and disables the input', () => {
      render(<NamespaceField {...singleProps} />);

      const input = screen.getByRole('textbox', { name: /Namespace/ });
      expect(input).toHaveValue('team-a');
      expect(input).toBeDisabled();
    });
  });

  describe('developer with multiple namespaces', () => {
    const multiProps = {
      ...defaultProps,
      canCreateNamespaces: false,
      namespaces: ['team-a', 'team-b'],
    };

    it('renders a dropdown of the accessible namespaces', () => {
      render(<NamespaceField {...multiProps} />);

      expect(screen.getByRole('combobox', { name: /Namespace/ })).toBeInTheDocument();
      expect(screen.getByRole('option', { name: 'team-a' })).toBeInTheDocument();
      expect(screen.getByRole('option', { name: 'team-b' })).toBeInTheDocument();
    });

    it('filters system namespaces out of the dropdown', () => {
      render(
        <NamespaceField
          {...multiProps}
          namespaces={['team-a', 'openshift-monitoring', 'kube-system', 'team-b']}
        />,
      );

      expect(screen.getByRole('option', { name: 'team-a' })).toBeInTheDocument();
      expect(screen.getByRole('option', { name: 'team-b' })).toBeInTheDocument();
      expect(
        screen.queryByRole('option', { name: 'openshift-monitoring' }),
      ).not.toBeInTheDocument();
      expect(screen.queryByRole('option', { name: 'kube-system' })).not.toBeInTheDocument();
    });

    it('calls onChange when a namespace is selected', async () => {
      const user = userEvent.setup();
      render(<NamespaceField {...multiProps} />);

      await user.selectOptions(screen.getByRole('combobox', { name: /Namespace/ }), 'team-a');

      expect(onChange).toHaveBeenCalledWith('team-a');
    });
  });

  describe('loading', () => {
    it('does not render a control while loading', () => {
      render(<NamespaceField {...defaultProps} loading />);

      expect(screen.queryByRole('textbox', { name: /Namespace/ })).not.toBeInTheDocument();
      expect(screen.queryByRole('combobox', { name: /Namespace/ })).not.toBeInTheDocument();
    });
  });
});
