import { K8sResourceCommon } from '@openshift-console/dynamic-plugin-sdk';
import { ClusterError } from '../../common/errors';
import { FunctionSource, WorkflowRun } from '../../common/types';

export interface Function {
  readonly owner?: string;
  readonly repoName?: string;
  readonly source: FunctionSource;
  readonly status: FunctionStatus;
  readonly routeURL?: string;
  readonly name?: string;
  readonly namespace?: string;
  readonly runtime?: string;
  readonly replicas?: number;
  readonly mainResource?: K8sResourceCommon;
  readonly error?: string;
}

export interface FunctionStatus {
  cluster: ClusterStatus;
  workflow: WorkflowRun;
}

export interface ClusterStatus {
  status: 'NotDeployed' | 'Deploying' | 'Running' | 'ScaledToZero' | 'Undeploying' | 'Error';
  error?: ClusterError;
}

/** Returns a unique key for a function. Entries with a name and namespace are
 * keyed by namespace/name, entries without one (func.yaml unreadable or
 * unparseable) by owner/repoName. The ~ prefix on the latter prevents collisions
 * when a K8s namespace happens to match a GitHub owner name (e.g. both produce
 * "alice/my-func"). Keying on the name rather than on error means an entry
 * that has an error but still parsed, such as a function whose namespace is
 * missing from the cluster, keeps matching its cluster counterpart. */
export function functionKey(
  fn: Pick<Function, 'owner' | 'repoName' | 'namespace' | 'name'>,
): string {
  return fn.name && fn.namespace ? `${fn.namespace}/${fn.name}` : `~${fn.owner}/${fn.repoName}`;
}
