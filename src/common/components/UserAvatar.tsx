import {
  Alert,
  Button,
  Divider,
  Dropdown,
  DropdownItem,
  DropdownList,
  Flex,
  FlexItem,
  Form,
  FormGroup,
  FormHelperText,
  HelperText,
  HelperTextItem,
  MenuToggle,
  MenuToggleElement,
  Modal,
  ModalBody,
  ModalFooter,
  ModalHeader,
  TextInput,
  Tooltip,
} from '@patternfly/react-core';
import { GithubIcon, KeyIcon, UserIcon } from '@patternfly/react-icons';
import { Ref, useContext, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { AuthContext } from '../context/AuthProvider';
import { useSessionService } from '../services/session/useSessionService';
import { errorMessage } from '../utils/utils';

interface UserAvatarProps {
  enableReconnect: boolean;
}

export function UserAvatar({ enableReconnect }: UserAvatarProps) {
  const { t } = useTranslation('plugin__console-functions-plugin');
  const { user, isConnected, isModalOpen, openModal, closeModal, login, disconnect } =
    useUserAvatar(enableReconnect);

  if (isConnected) {
    return <ConnectedMenu name={user.name} onDisconnect={disconnect} />;
  }

  return (
    <>
      <Button
        variant="link"
        icon={<KeyIcon />}
        onClick={enableReconnect ? openModal : undefined}
        isDisabled={!enableReconnect}
        style={!enableReconnect ? { cursor: 'default' } : undefined}
      >
        {t('Connect to GitHub')}
      </Button>
      <PatModal isOpen={isModalOpen} onClose={closeModal} onConnect={login} />
    </>
  );
}

function useUserAvatar(enableReconnect: boolean) {
  const sessionService = useSessionService();
  const { user, isAuthenticated, onLogin, onLogout } = useContext(AuthContext);
  const [isModalOpen, setIsModalOpen] = useState(
    () => enableReconnect && !sessionService.isSessionActive(),
  );

  const login = async (pat: string) => {
    const authUser = await sessionService.login(pat);
    setIsModalOpen(false);
    onLogin(authUser);
  };

  const openModal = () => setIsModalOpen(true);
  const closeModal = () => setIsModalOpen(false);

  return {
    user,
    isConnected: isAuthenticated && Boolean(user.name),
    isModalOpen,
    openModal,
    closeModal,
    login,
    disconnect: onLogout,
  };
}

interface ConnectedMenuProps {
  name: string;
  onDisconnect: () => Promise<void>;
}

function ConnectedMenu({ name, onDisconnect }: ConnectedMenuProps) {
  const { t } = useTranslation('plugin__console-functions-plugin');
  const [isOpen, setIsOpen] = useState(false);

  return (
    <Dropdown
      isOpen={isOpen}
      onSelect={() => setIsOpen(false)}
      onOpenChange={setIsOpen}
      popperProps={{ position: 'right' }}
      toggle={(toggleRef: Ref<MenuToggleElement>) => (
        <MenuToggle
          ref={toggleRef}
          variant="plainText"
          icon={<UserIcon />}
          isExpanded={isOpen}
          onClick={() => setIsOpen((open) => !open)}
          data-test="user-menu-toggle"
        >
          {name}
        </MenuToggle>
      )}
    >
      <DropdownList>
        <DropdownItem value="disconnect" onClick={onDisconnect} data-test="disconnect-item">
          {t('Disconnect')}
        </DropdownItem>
      </DropdownList>
    </Dropdown>
  );
}

interface PatModalProps {
  isOpen: boolean;
  onClose: () => void;
  onConnect: (pat: string) => Promise<void>;
}

function PatModal({ isOpen, onClose, onConnect }: PatModalProps) {
  const { t } = useTranslation('plugin__console-functions-plugin');
  const { pat, isValidating, error, setPat, handleConnect, handleClose } = usePatModal(
    onClose,
    onConnect,
  );

  return (
    <Modal isOpen={isOpen} onClose={isValidating ? undefined : handleClose} variant="small">
      <ModalHeader title={t('Connect to GitHub')} />
      <ModalBody>
        {error && (
          <Alert variant="danger" title={error} isInline style={{ marginBottom: '1rem' }} />
        )}
        <Tooltip content={t('Coming soon')}>
          <Button
            className="pf-v6-u-my-md"
            variant="secondary"
            icon={<GithubIcon />}
            isAriaDisabled
            isBlock
            data-test="oauth-button"
          >
            {t('Sign in with GitHub')}
          </Button>
        </Tooltip>
        <Flex
          className="pf-v6-u-my-md"
          alignItems={{ default: 'alignItemsCenter' }}
          spaceItems={{ default: 'spaceItemsSm' }}
        >
          <FlexItem flex={{ default: 'flex_1' }}>
            <Divider />
          </FlexItem>
          <FlexItem>{t('or')}</FlexItem>
          <FlexItem flex={{ default: 'flex_1' }}>
            <Divider />
          </FlexItem>
        </Flex>
        <Form
          onSubmit={(e) => {
            e.preventDefault();
            if (pat && !isValidating) handleConnect();
          }}
        >
          <FormGroup label={t('Personal Access Token')} fieldId="pat-input">
            <TextInput
              id="pat-input"
              type="password"
              value={pat}
              onChange={(_, value) => setPat(value)}
            />
            <FormHelperText>
              <HelperText>
                <HelperTextItem>
                  {t('Enter your GitHub Personal Access Token to connect your repositories.')}
                </HelperTextItem>
              </HelperText>
            </FormHelperText>
          </FormGroup>
        </Form>
      </ModalBody>
      <ModalFooter>
        <Button
          variant="primary"
          onClick={handleConnect}
          isDisabled={!pat || isValidating}
          isLoading={isValidating}
        >
          {t('Connect')}
        </Button>
        <Button variant="link" onClick={handleClose} isDisabled={isValidating}>
          {t('Cancel')}
        </Button>
      </ModalFooter>
    </Modal>
  );
}

function usePatModal(onClose: () => void, onConnect: (pat: string) => Promise<void>) {
  const [pat, setPat] = useState('');
  const [isValidating, setIsValidating] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleConnect = async () => {
    setIsValidating(true);
    setError(null);
    try {
      await onConnect(pat);
      setPat('');
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setIsValidating(false);
    }
  };

  const handleClose = () => {
    setPat('');
    setError(null);
    onClose();
  };

  return { pat, isValidating, error, setPat, handleConnect, handleClose };
}
