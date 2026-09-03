import {
  Alert,
  FormGroup,
  FormSelect,
  FormSelectOption,
  Skeleton,
  TextInput,
} from '@patternfly/react-core';
import { useTranslation } from 'react-i18next';
import { isSystemNamespace } from '../../../common/utils/utils';
import { NamespaceRole } from '../../../common/clients/namespace';

interface NamespaceFieldProps {
  role: NamespaceRole;
  namespaces: string[];
  loading: boolean;
  value: string;
  onChange: (namespace: string) => void;
}

export function NamespaceField({
  role,
  namespaces,
  loading,
  value,
  onChange,
}: NamespaceFieldProps) {
  const { t } = useTranslation('plugin__console-functions-plugin');

  if (loading) {
    return (
      <FormGroup label={t('Namespace')}>
        <Skeleton screenreaderText={t('Loading namespaces')} width="100%" />
      </FormGroup>
    );
  }

  if (role === 'developer-none') {
    return (
      <FormGroup label={t('Namespace')}>
        <Alert
          variant="info"
          isInline
          title={t(
            'You do not have access to any namespaces. Contact an administrator to get access to a namespace for your functions.',
          )}
        />
      </FormGroup>
    );
  }

  return (
    <FormGroup label={t('Namespace')} isRequired fieldId="namespace">
      {role === 'developer-single' ? (
        <TextInput id="namespace" isRequired isDisabled value={value} aria-label={t('Namespace')} />
      ) : role === 'developer-multi' ? (
        <FormSelect
          id="namespace"
          value={value}
          onChange={(_, val) => onChange(val)}
          aria-label={t('Namespace')}
        >
          <FormSelectOption value="" label={t('Select...')} isPlaceholder />
          {namespaces.map((ns) => (
            <FormSelectOption key={ns} value={ns} label={ns} />
          ))}
        </FormSelect>
      ) : (
        <TextInput
          id="namespace"
          isRequired
          value={value}
          onChange={(_, val) => onChange(val)}
          aria-label={t('Namespace')}
        />
      )}
      {isSystemNamespace(value) && (
        <Alert
          variant="warning"
          isInline
          title={t(
            'Functions should not be deployed to a system namespace. Deployment there is likely to fail. Create a new namespace for your functions instead.',
          )}
          className="pf-v6-u-mt-sm"
        />
      )}
    </FormGroup>
  );
}
