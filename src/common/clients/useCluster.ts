import {
  K8sResourceKind,
  useAccessReview,
  useK8sWatchResource,
  WatchK8sResource,
} from '@openshift-console/dynamic-plugin-sdk';
import { useMemo } from 'react';
import { isNotFoundError } from '../utils/utils';
import {
  ClusterFunction,
  FUNCTION_NAME_LABEL,
  FunctionStatus,
  K8sKeyedResource,
  REVISION_LABEL,
} from '../types';
import { NamespaceRole, resolveNamespace } from './namespace';

interface ClusterOptions {
  // The namespace options (role, accessible namespaces) require an access review and a
  // Project watch that only the create form needs, so they stay off by default.
  withNamespaceOptions?: boolean;
}

export function useCluster(
  functionNames: string[] = [],
  namespace?: string,
  options: ClusterOptions = {},
): {
  functions: ReadonlyMap<string, ClusterFunction>;
  secrets: K8sKeyedResource[];
  configMaps: K8sKeyedResource[];
  loaded: boolean;
  error: Error;
  role: NamespaceRole;
  namespaces: string[];
  namespacesLoading: boolean;
} {
  const { withNamespaceOptions = false } = options;

  // An empty group+resource with the third arg set makes the SDK skip the review, so a
  // consumer that does not need namespace options never fires a SelfSubjectAccessReview.
  const [canCreateNamespaces, accessLoading] = useAccessReview(
    withNamespaceOptions ? { group: '', resource: 'namespaces', verb: 'create' } : {},
    undefined,
    true,
  );

  const projectConfig = useMemo(
    () =>
      withNamespaceOptions
        ? {
            groupVersionKind: { group: 'project.openshift.io', version: 'v1', kind: 'Project' },
            isList: true,
          }
        : null,
    [withNamespaceOptions],
  );

  const [projects, projectsLoaded] = useK8sWatchResource<K8sResourceKind[]>(projectConfig);

  const namespaces = useMemo(
    () =>
      (projects ?? [])
        .map((p) => p.metadata?.name)
        .filter((name): name is string => Boolean(name))
        .sort(),
    [projects],
  );

  const namespacesLoading = withNamespaceOptions && (accessLoading || !projectsLoaded);

  const role: NamespaceRole = canCreateNamespaces
    ? 'admin'
    : namespaces.length === 0
      ? 'developer-none'
      : namespaces.length === 1
        ? 'developer-single'
        : 'developer-multi';

  // A single-namespace developer has no editable control, so watch their one namespace
  // rather than the (empty) typed value.
  const effectiveNamespace = withNamespaceOptions
    ? resolveNamespace(role, namespaces, namespace ?? '')
    : namespace;

  const knSvcConfig = useMemo(
    () => newKsvcWatchConfig(functionNames, namespace),
    [functionNames, namespace],
  );
  const depConfig = useMemo(
    () => newDeploymentWatchConfig(functionNames, namespace),
    [functionNames, namespace],
  );

  const secretConfig = useMemo(() => newSecretConfig(effectiveNamespace), [effectiveNamespace]);
  const configMapConfig = useMemo(
    () => newConfigMapConfig(effectiveNamespace),
    [effectiveNamespace],
  );

  const [knSvcs, knLoaded, knError] = useK8sWatchResource<K8sResourceKind[]>(knSvcConfig);
  const [deps, depLoaded, depError] = useK8sWatchResource<K8sResourceKind[]>(depConfig);
  const [rawSecrets, secretLoaded, secretError] =
    useK8sWatchResource<K8sResourceKind[]>(secretConfig);
  const [rawConfigMaps, cmLoaded, cmError] =
    useK8sWatchResource<K8sResourceKind[]>(configMapConfig);

  const functions = useMemo(() => {
    const safeKnSvcs = knLoaded ? (knSvcs ?? []) : [];
    const safeDeps = depLoaded ? (deps ?? []) : [];
    return listKnativeClusterFunctions(safeKnSvcs, safeDeps);
  }, [knSvcs, knLoaded, deps, depLoaded]);

  const secrets = useMemo(() => toKeyedResources(rawSecrets), [rawSecrets]);
  const configMaps = useMemo(() => toKeyedResources(rawConfigMaps), [rawConfigMaps]);

  const loaded = knLoaded && depLoaded && (!effectiveNamespace || (secretLoaded && cmLoaded));

  // A not-found watch error just means the namespace does not exist yet (an admin can type
  // one that has not been created); swallow it so it is not surfaced as a scary error.
  const namespaceScopedError =
    (isNotFoundError(secretError) ? null : secretError) ||
    (isNotFoundError(cmError) ? null : cmError) ||
    null;

  return {
    functions,
    secrets,
    configMaps,
    loaded,
    error: knError || depError || namespaceScopedError,
    role,
    namespaces,
    namespacesLoading,
  };
}

function newKsvcWatchConfig(functionNames: string[], namespace?: string): WatchK8sResource | null {
  return functionNames.length > 0
    ? newWatchConfigWithSelector(functionNames, 'serving.knative.dev', 'Service', namespace)
    : null;
}

function newWatchConfigWithSelector(
  functionNames: string[],
  group: string,
  kind: string,
  namespace?: string,
): WatchK8sResource {
  return {
    groupVersionKind: { group, version: 'v1', kind },
    isList: true,
    namespace,
    selector: {
      matchExpressions: [{ key: FUNCTION_NAME_LABEL, operator: 'In', values: functionNames }],
    },
  };
}

function newDeploymentWatchConfig(
  functionNames: string[],
  namespace?: string,
): WatchK8sResource | null {
  return functionNames.length > 0
    ? newWatchConfigWithSelector(functionNames, 'apps', 'Deployment', namespace)
    : null;
}

function newSecretConfig(namespace?: string): WatchK8sResource | null {
  return namespace ? newDataWatchConfig('Secret', namespace) : null;
}

function newConfigMapConfig(namespace?: string): WatchK8sResource | null {
  return namespace ? newDataWatchConfig('ConfigMap', namespace) : null;
}

function newDataWatchConfig(kind: string, namespace?: string): WatchK8sResource {
  return {
    groupVersionKind: { version: 'v1', kind },
    namespace,
    isList: true,
  };
}

function listKnativeClusterFunctions(
  knSvcs: K8sResourceKind[],
  deployments: K8sResourceKind[],
): ReadonlyMap<string, ClusterFunction> {
  const entries = knSvcs.map((ksvc): [string, ClusterFunction] => {
    const name = ksvc.metadata?.labels?.[FUNCTION_NAME_LABEL] ?? ksvc.metadata?.name ?? '';
    const namespace = ksvc.metadata?.namespace ?? '';
    const latestRevision = ksvc.status?.latestReadyRevisionName;

    const nsDeployments = deployments.filter((d) => d.metadata?.namespace === namespace);
    const deployment = latestRevision
      ? nsDeployments.find((d) => d.metadata?.labels?.[REVISION_LABEL] === latestRevision)
      : nsDeployments.find((d) => d.metadata?.labels?.[FUNCTION_NAME_LABEL] === name);

    return [
      `${namespace}/${name}`,
      {
        name,
        namespace,
        status: deriveKnativeStatus(ksvc, deployment),
        url: ksvc.status?.url ?? '',
        replicas: deployment?.status?.readyReplicas ?? 0,
        mainResource: ksvc,
      },
    ];
  });

  return new Map(entries);
}

function deriveKnativeStatus(
  ksvc: K8sResourceKind,
  deployment: K8sResourceKind | undefined,
): FunctionStatus {
  if (!deployment) return 'Deploying';

  const conditions = ksvc.status?.conditions ?? [];
  const ready = conditions.find((c: { type: string }) => c.type === 'Ready');
  if (!ready) return 'Deploying';

  if (ready.status === 'True') {
    const desired = deployment.spec?.replicas ?? 0;
    const readyReplicas = deployment.status?.readyReplicas ?? 0;
    if (desired === 0 && readyReplicas === 0) return 'ScaledToZero';
    return 'Running';
  }

  if (ready.status === 'False') return 'Error';

  return 'Deploying';
}

function toKeyedResources(resources: K8sResourceKind[]): K8sKeyedResource[] {
  return (resources ?? [])
    .filter((r) => r.metadata?.name)
    .map((r) => ({
      name: r.metadata!.name!,
      keys: r.data ? Object.keys(r.data) : [],
    }));
}
