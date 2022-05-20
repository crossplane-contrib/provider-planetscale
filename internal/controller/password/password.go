/*
Copyright 2022 The Crossplane Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package password

import (
	"context"

	"github.com/pkg/errors"
	"github.com/planetscale/planetscale-go/planetscale"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/pkg/connection"
	"github.com/crossplane/crossplane-runtime/pkg/controller"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"

	"github.com/crossplane/provider-planetscale/apis/branch/v1alpha1"
	apisv1alpha1 "github.com/crossplane/provider-planetscale/apis/v1alpha1"
	"github.com/crossplane/provider-planetscale/internal/controller/features"
)

const (
	errNotPassword  = "managed resource is not a Password custom resource"
	errTrackPCUsage = "cannot track ProviderConfig usage"
	errGetPC        = "cannot get ProviderConfig"
	errGetCreds     = "cannot get credentials"

	errNewClient = "cannot create new Service"
)

// A PlanetScaleService does nothing.
type PlanetScaleService struct {
	pCLI *planetscale.Client
}

var (
	newPlanetScaleService = func(creds []byte) (*PlanetScaleService, error) {
		c, err := planetscale.NewClient(planetscale.WithAccessToken(string(creds)))
		return &PlanetScaleService{
			pCLI: c,
		}, err
	}
)

// Setup adds a controller that reconciles Password managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.PasswordGroupKind)

	cps := []managed.ConnectionPublisher{managed.NewAPISecretPublisher(mgr.GetClient(), mgr.GetScheme())}
	if o.Features.Enabled(features.EnableAlphaExternalSecretStores) {
		cps = append(cps, connection.NewDetailsManager(mgr.GetClient(), apisv1alpha1.StoreConfigGroupVersionKind))
	}

	r := managed.NewReconciler(mgr,
		resource.ManagedKind(v1alpha1.PasswordGroupVersionKind),
		managed.WithExternalConnecter(&connector{
			kube:         mgr.GetClient(),
			usage:        resource.NewProviderConfigUsageTracker(mgr.GetClient(), &apisv1alpha1.ProviderConfigUsage{}),
			newServiceFn: newPlanetScaleService}),
		managed.WithLogger(o.Logger.WithValues("controller", name)),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
		managed.WithConnectionPublishers(cps...))

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		For(&v1alpha1.Password{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

// A connector is expected to produce an ExternalClient when its Connect method
// is called.
type connector struct {
	kube         client.Client
	usage        resource.Tracker
	newServiceFn func(creds []byte) (*PlanetScaleService, error)
}

// Connect typically produces an ExternalClient by:
// 1. Tracking that the managed resource is using a ProviderConfig.
// 2. Getting the managed resource's ProviderConfig.
// 3. Getting the credentials specified by the ProviderConfig.
// 4. Using the credentials to form a client.
func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.Password)
	if !ok {
		return nil, errors.New(errNotPassword)
	}

	if err := c.usage.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errTrackPCUsage)
	}

	pc := &apisv1alpha1.ProviderConfig{}
	if err := c.kube.Get(ctx, types.NamespacedName{Name: cr.GetProviderConfigReference().Name}, pc); err != nil {
		return nil, errors.Wrap(err, errGetPC)
	}

	cd := pc.Spec.Credentials
	data, err := resource.CommonCredentialExtractor(ctx, cd.Source, c.kube, cd.CommonCredentialSelectors)
	if err != nil {
		return nil, errors.Wrap(err, errGetCreds)
	}

	svc, err := c.newServiceFn(data)
	if err != nil {
		return nil, errors.Wrap(err, errNewClient)
	}

	return &external{service: svc}, nil
}

// An ExternalClient observes, then either creates, updates, or deletes an
// external resource to ensure it reflects the managed resource's desired state.
type external struct {
	// A 'client' used to connect to the external resource API. In practice this
	// would be something like an AWS SDK client.
	service *PlanetScaleService
}

func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.Password)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotPassword)
	}

	p, err := c.service.pCLI.Passwords.Get(ctx, &planetscale.GetDatabaseBranchPasswordRequest{
		Organization: cr.Spec.ForProvider.Organization,
		Database:     *cr.Spec.ForProvider.Database,
		Branch:       cr.Spec.ForProvider.Branch,
		DisplayName:  cr.Name,
		PasswordId:   meta.GetExternalName(cr),
	})

	if pErr, ok := err.(*planetscale.Error); ok && pErr.Code == planetscale.ErrNotFound {
		return managed.ExternalObservation{
			ResourceExists: false,
		}, nil
	}

	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: p.Name != cr.GetName(),
	}, err
}

func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.Password)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotPassword)
	}

	p, err := c.service.pCLI.Passwords.Create(ctx, &planetscale.DatabaseBranchPasswordRequest{
		Organization: cr.Spec.ForProvider.Organization,
		Database:     *cr.Spec.ForProvider.Database,
		Branch:       cr.Spec.ForProvider.Branch,
		DisplayName:  cr.Name,
	})
	if err != nil {
		return managed.ExternalCreation{}, err
	}

	meta.SetExternalName(cr, p.PublicID)
	cr.Status.AtProvider.ID = p.PublicID

	return managed.ExternalCreation{
		// Optionally return any details that may be required to connect to the
		// external resource. These will be stored as the connection secret.
		ConnectionDetails: managed.ConnectionDetails{
			"host":     []byte(p.Branch.AccessHostURL),
			"username": []byte(p.PublicID),
			"password": []byte(p.PlainText),
			"database": []byte(*cr.Spec.ForProvider.Database),
		},
	}, err
}

func (c *external) Update(_ context.Context, _ resource.Managed) (managed.ExternalUpdate, error) {
	// No update required for this resource
	return managed.ExternalUpdate{}, nil
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) error {
	cr, ok := mg.(*v1alpha1.Password)
	if !ok {
		return errors.New(errNotPassword)
	}

	return c.service.pCLI.Passwords.Delete(ctx, &planetscale.DeleteDatabaseBranchPasswordRequest{
		Organization: cr.Spec.ForProvider.Organization,
		Database:     *cr.Spec.ForProvider.Database,
		Branch:       cr.Spec.ForProvider.Branch,
		DisplayName:  cr.Name,
		PasswordId:   meta.GetExternalName(cr),
	})
}
