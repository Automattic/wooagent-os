<?php
/**
 * Product abilities for WooAgent.
 *
 * wooagent-products/list, wooagent-products/get, wooagent-products/update
 */

if ( ! defined( 'ABSPATH' ) ) {
	exit;
}

function wooagent_companion_register_product_abilities(): void {
	wp_register_ability(
		'wooagent-products/list',
		array(
			'label'               => __( 'List products', 'wooagent-companion' ),
			'description'         => __( 'List WooCommerce products with optional filters. Returns a lightweight summary per product.', 'wooagent-companion' ),
			'input_schema'        => array(
				'type'       => 'object',
				'properties' => array(
					'status'   => array(
						'type'        => 'string',
						'enum'        => array( 'any', 'publish', 'draft', 'pending', 'private' ),
						'default'     => 'publish',
						'description' => 'Product status filter.',
					),
					'per_page' => array(
						'type'        => 'integer',
						'minimum'     => 1,
						'maximum'     => 100,
						'default'     => 25,
						'description' => 'Items per page.',
					),
					'page'     => array(
						'type'    => 'integer',
						'minimum' => 1,
						'default' => 1,
					),
					'search'   => array(
						'type'        => 'string',
						'description' => 'Text search across product name and SKU.',
					),
					'orderby'  => array(
						'type'        => 'string',
						'enum'        => array( 'menu_order', 'date', 'date_modified', 'total_sales', 'title' ),
						'default'     => 'menu_order',
						'description' => 'Sort key. "total_sales" with order="asc" surfaces slow movers; with order="desc" surfaces bestsellers. "date_modified" with order="asc" surfaces stale products.',
					),
					'order'    => array(
						'type'        => 'string',
						'enum'        => array( 'asc', 'desc' ),
						'default'     => 'desc',
						'description' => 'Sort direction. Ignored when orderby="menu_order" (WooCommerce default applies).',
					),
				),
				'additionalProperties' => false,
			),
			'output_schema'       => array(
				'type'       => 'object',
				'properties' => array(
					'products' => array(
						'type'  => 'array',
						'items' => array(
							'type'       => 'object',
							'properties' => array(
								'id'                   => array( 'type' => 'integer' ),
								'name'                 => array( 'type' => 'string' ),
								'slug'                 => array( 'type' => 'string' ),
								'sku'                  => array( 'type' => 'string' ),
								'status'               => array( 'type' => 'string' ),
								'type'                 => array( 'type' => 'string' ),
								'regular_price'        => array( 'type' => 'string' ),
								'sale_price'           => array( 'type' => 'string' ),
								'on_sale'              => array( 'type' => 'boolean' ),
								'description_length'   => array( 'type' => 'integer' ),
								'short_description_length' => array( 'type' => 'integer' ),
								'featured'             => array( 'type' => 'boolean' ),
								'date_modified'        => array( 'type' => 'string' ),
								'total_sales'          => array( 'type' => 'integer', 'description' => 'Lifetime units sold per WooCommerce. 0 for products that have never been ordered.' ),
							),
						),
					),
					'total'    => array( 'type' => 'integer' ),
				),
				'required'   => array( 'products', 'total' ),
			),
			'category'            => 'wooagent-products',
			'execute_callback'    => 'wooagent_products_list_execute',
			'permission_callback' => 'wooagent_products_read_permission',
			'meta'                => array(
				'show_in_rest' => true,
				'mcp'          => array( 'public' => true ),
				'annotations'  => array( 'readonly' => true, 'idempotent' => true ),
			),
		)
	);

	wp_register_ability(
		'wooagent-products/get',
		array(
			'label'               => __( 'Get product', 'wooagent-companion' ),
			'description'         => __( 'Read a single product including full description, short description, and SEO meta.', 'wooagent-companion' ),
			'input_schema'        => array(
				'type'       => 'object',
				'properties' => array(
					'id' => array(
						'type'        => 'integer',
						'minimum'     => 1,
						'description' => 'Product ID.',
					),
				),
				'required'   => array( 'id' ),
				'additionalProperties' => false,
			),
			'output_schema'       => array(
				'type'       => 'object',
				'properties' => array(
					'id'                => array( 'type' => 'integer' ),
					'name'              => array( 'type' => 'string' ),
					'slug'              => array( 'type' => 'string' ),
					'sku'               => array( 'type' => 'string' ),
					'status'            => array( 'type' => 'string' ),
					'type'              => array( 'type' => 'string' ),
					'regular_price'     => array( 'type' => 'string' ),
					'sale_price'        => array( 'type' => 'string' ),
					'description'       => array( 'type' => 'string' ),
					'short_description' => array( 'type' => 'string' ),
					'categories'        => array(
						'type'  => 'array',
						'items' => array(
							'type'       => 'object',
							'properties' => array(
								'id'   => array( 'type' => 'integer' ),
								'name' => array( 'type' => 'string' ),
							),
						),
					),
					'tags'              => array(
						'type'  => 'array',
						'items' => array( 'type' => 'string' ),
					),
					'meta_title'        => array( 'type' => 'string' ),
					'meta_description'  => array( 'type' => 'string' ),
					'featured'          => array( 'type' => 'boolean' ),
					'date_modified'     => array( 'type' => 'string' ),
					'image_url'         => array( 'type' => 'string' ),
					'image_alt'         => array( 'type' => 'string' ),
				),
				'required'   => array( 'id', 'name', 'status' ),
			),
			'category'            => 'wooagent-products',
			'execute_callback'    => 'wooagent_products_get_execute',
			'permission_callback' => 'wooagent_products_read_permission',
			'meta'                => array(
				'show_in_rest' => true,
				'mcp'          => array( 'public' => true ),
				'annotations'  => array( 'readonly' => true, 'idempotent' => true ),
			),
		)
	);

	wp_register_ability(
		'wooagent-products/list-categories',
		array(
			'label'               => __( 'List product categories', 'wooagent-companion' ),
			'description'         => __( 'Return the store\'s product category taxonomy as a flat list. Used by Marketing to detect uncategorized clusters and propose category assignments.', 'wooagent-companion' ),
			'input_schema'        => array(
				'type'       => 'object',
				'properties' => array(
					'hide_empty' => array(
						'type'        => 'boolean',
						'default'     => false,
						'description' => 'When true, omit categories with zero products. Default false so callers see the full taxonomy including new/empty branches.',
					),
				),
				'additionalProperties' => false,
			),
			'output_schema'       => array(
				'type'       => 'object',
				'properties' => array(
					'categories' => array(
						'type'  => 'array',
						'items' => array(
							'type'       => 'object',
							'properties' => array(
								'id'        => array( 'type' => 'integer' ),
								'name'      => array( 'type' => 'string' ),
								'slug'      => array( 'type' => 'string' ),
								'parent_id' => array( 'type' => 'integer', 'description' => '0 when the category is at the root.' ),
								'count'     => array( 'type' => 'integer', 'description' => 'Number of products assigned (including drafts).' ),
							),
							'required'   => array( 'id', 'name', 'slug', 'parent_id', 'count' ),
						),
					),
					'total'      => array( 'type' => 'integer' ),
				),
				'required'   => array( 'categories', 'total' ),
			),
			'category'            => 'wooagent-products',
			'execute_callback'    => 'wooagent_products_list_categories_execute',
			'permission_callback' => 'wooagent_products_read_permission',
			'meta'                => array(
				'show_in_rest' => true,
				'mcp'          => array( 'public' => true ),
				'annotations'  => array( 'readonly' => true, 'idempotent' => true ),
			),
		)
	);

	wp_register_ability(
		'wooagent-products/update',
		array(
			'label'               => __( 'Update product', 'wooagent-companion' ),
			'description'         => __( 'Update any subset of editable product fields. Only the fields provided are modified.', 'wooagent-companion' ),
			'input_schema'        => array(
				'type'       => 'object',
				'properties' => array(
					'id'                => array( 'type' => 'integer', 'minimum' => 1 ),
					'name'              => array( 'type' => 'string' ),
					'description'       => array( 'type' => 'string' ),
					'short_description' => array( 'type' => 'string' ),
					'regular_price'     => array( 'type' => 'string', 'description' => 'Decimal string, e.g., "19.99".' ),
					'sale_price'        => array( 'type' => 'string' ),
					'status'            => array( 'type' => 'string', 'enum' => array( 'publish', 'draft', 'pending', 'private' ) ),
					'meta_title'        => array( 'type' => 'string' ),
					'meta_description'  => array( 'type' => 'string' ),
				),
				'required'   => array( 'id' ),
				'additionalProperties' => false,
			),
			'output_schema'       => array(
				'type'       => 'object',
				'properties' => array(
					'id'            => array( 'type' => 'integer' ),
					'updated_fields' => array( 'type' => 'array', 'items' => array( 'type' => 'string' ) ),
					'date_modified' => array( 'type' => 'string' ),
				),
				'required'   => array( 'id', 'updated_fields' ),
			),
			'category'            => 'wooagent-products',
			'execute_callback'    => 'wooagent_products_update_execute',
			'permission_callback' => 'wooagent_products_write_permission',
			'meta'                => array(
				'show_in_rest' => true,
				'mcp'          => array( 'public' => true ),
				'annotations'  => array( 'readonly' => false, 'destructive' => false, 'idempotent' => false ),
			),
		)
	);

	wp_register_ability(
		'wooagent-products/variations-list',
		array(
			'label'               => __( 'List product variations', 'wooagent-companion' ),
			'description'         => __( 'List all enabled, priced variations of a variable product. Returns id, attribute label, regular_price, sale_price, and stock_status per variation.', 'wooagent-companion' ),
			'input_schema'        => array(
				'type'       => 'object',
				'properties' => array(
					'product_id' => array(
						'type'        => 'integer',
						'minimum'     => 1,
						'description' => 'Variable parent product ID.',
					),
				),
				'required'             => array( 'product_id' ),
				'additionalProperties' => false,
			),
			'output_schema'       => array(
				'type'       => 'object',
				'properties' => array(
					'parent_id'  => array( 'type' => 'integer' ),
					'variations' => array(
						'type'  => 'array',
						'items' => array(
							'type'       => 'object',
							'properties' => array(
								'id'               => array( 'type' => 'integer' ),
								'attributes_label' => array( 'type' => 'string', 'description' => 'Human-readable attribute summary, e.g., "Small / Blue".' ),
								'regular_price'    => array( 'type' => 'string' ),
								'sale_price'       => array( 'type' => 'string' ),
								'stock_status'     => array( 'type' => 'string' ),
								'menu_order'       => array( 'type' => 'integer' ),
							),
							'required'   => array( 'id', 'attributes_label', 'regular_price', 'stock_status' ),
						),
					),
				),
				'required'   => array( 'parent_id', 'variations' ),
			),
			'category'            => 'wooagent-products',
			'execute_callback'    => 'wooagent_products_variations_list_execute',
			'permission_callback' => 'wooagent_products_read_permission',
			'meta'                => array(
				'show_in_rest' => true,
				'mcp'          => array( 'public' => true ),
				'annotations'  => array( 'readonly' => true, 'idempotent' => true ),
			),
		)
	);
}

function wooagent_products_read_permission(): bool {
	return current_user_can( 'read_private_products' ) || current_user_can( 'edit_products' );
}

function wooagent_products_write_permission(): bool {
	return current_user_can( 'edit_products' );
}

function wooagent_products_list_execute( array $args ) {
	if ( ! class_exists( 'WC_Product_Query' ) ) {
		return new WP_Error( 'wooagent_woocommerce_missing', __( 'WooCommerce is not active.', 'wooagent-companion' ) );
	}

	$query_args = array(
		'status'   => $args['status'] ?? 'publish',
		'limit'    => $args['per_page'] ?? 25,
		'page'     => $args['page'] ?? 1,
		'paginate' => true,
	);

	if ( 'any' === $query_args['status'] ) {
		$query_args['status'] = array( 'publish', 'draft', 'pending', 'private' );
	}

	if ( ! empty( $args['search'] ) ) {
		$query_args['s'] = $args['search'];
	}

	// orderby: pass-through for the names WC's WC_Product_Query understands
	// natively (date, title, menu_order). date_modified + total_sales need
	// to be translated to meta_value_num lookups since WC_Product_Query
	// doesn't accept them as first-class orderby keys.
	$orderby = $args['orderby'] ?? 'menu_order';
	$order   = strtoupper( $args['order'] ?? 'desc' );
	if ( ! in_array( $order, array( 'ASC', 'DESC' ), true ) ) {
		$order = 'DESC';
	}
	switch ( $orderby ) {
		case 'menu_order':
			// WC default; intentionally leave query_args alone so the
			// "menu_order title" multi-key WC default applies.
			break;
		case 'date':
		case 'title':
			$query_args['orderby'] = $orderby;
			$query_args['order']   = $order;
			break;
		case 'date_modified':
			$query_args['orderby'] = 'modified';
			$query_args['order']   = $order;
			break;
		case 'total_sales':
			$query_args['orderby']  = 'meta_value_num';
			$query_args['meta_key'] = 'total_sales'; // phpcs:ignore WordPress.DB.SlowDBQuery.slow_db_query_meta_key
			$query_args['order']    = $order;
			break;
	}

	$result = wc_get_products( $query_args );

	$products = array();
	foreach ( $result->products as $product ) {
		$products[] = wooagent_product_to_summary( $product );
	}

	return array(
		'products' => $products,
		'total'    => (int) $result->total,
	);
}

function wooagent_products_get_execute( array $args ) {
	$product = wc_get_product( (int) $args['id'] );
	if ( ! $product ) {
		return new WP_Error( 'wooagent_product_not_found', __( 'Product not found.', 'wooagent-companion' ), array( 'status' => 404 ) );
	}

	$categories = array();
	foreach ( $product->get_category_ids() as $cat_id ) {
		$term = get_term( $cat_id, 'product_cat' );
		if ( $term && ! is_wp_error( $term ) ) {
			$categories[] = array( 'id' => (int) $term->term_id, 'name' => $term->name );
		}
	}

	$tags = array();
	foreach ( $product->get_tag_ids() as $tag_id ) {
		$term = get_term( $tag_id, 'product_tag' );
		if ( $term && ! is_wp_error( $term ) ) {
			$tags[] = $term->name;
		}
	}

	$image_id  = (int) $product->get_image_id();
	$image_url = '';
	$image_alt = '';
	if ( $image_id > 0 ) {
		// 'medium' (max 300×300) keeps payload small; WP falls back to the
		// original upload if the size isn't registered on the site.
		$src = wp_get_attachment_image_src( $image_id, 'medium' );
		if ( is_array( $src ) && ! empty( $src[0] ) ) {
			$image_url = (string) $src[0];
			$image_alt = (string) get_post_meta( $image_id, '_wp_attachment_image_alt', true );
		}
	}

	return array(
		'id'                => $product->get_id(),
		'name'              => $product->get_name(),
		'slug'              => $product->get_slug(),
		'sku'               => $product->get_sku(),
		'status'            => $product->get_status(),
		'type'              => $product->get_type(),
		'regular_price'     => $product->get_regular_price(),
		'sale_price'        => $product->get_sale_price(),
		'description'       => $product->get_description(),
		'short_description' => $product->get_short_description(),
		'categories'        => $categories,
		'tags'              => $tags,
		'meta_title'        => (string) get_post_meta( $product->get_id(), '_yoast_wpseo_title', true ),
		'meta_description'  => (string) get_post_meta( $product->get_id(), '_yoast_wpseo_metadesc', true ),
		'featured'          => $product->is_featured(),
		'date_modified'     => $product->get_date_modified() ? $product->get_date_modified()->date( 'c' ) : '',
		'image_url'         => $image_url,
		'image_alt'         => $image_alt,
	);
}

function wooagent_products_variations_list_execute( array $args ) {
	$parent = wc_get_product( (int) $args['product_id'] );
	if ( ! $parent ) {
		return new WP_Error( 'wooagent_product_not_found', __( 'Product not found.', 'wooagent-companion' ), array( 'status' => 404 ) );
	}
	if ( ! $parent->is_type( 'variable' ) ) {
		return new WP_Error( 'wooagent_not_variable_product', __( 'Product is not a variable product.', 'wooagent-companion' ), array( 'status' => 422 ) );
	}

	$variations = array();
	foreach ( $parent->get_children() as $variation_id ) {
		$variation = wc_get_product( (int) $variation_id );
		if ( ! $variation || ! $variation->is_type( 'variation' ) ) {
			continue;
		}
		// Skip disabled variations — Pricing should never propose changes
		// to a variation that's not for sale.
		if ( 'publish' !== $variation->get_status() ) {
			continue;
		}

		// Build "Small / Blue" from attribute values. WC stores
		// attribute keys as pa_size, pa_color, attribute_pa_size, etc.;
		// get_variation_attributes() returns the keyed array.
		$attribute_values = array();
		foreach ( $variation->get_variation_attributes() as $key => $value ) {
			if ( '' === $value ) {
				continue;
			}
			// Strip 'attribute_' prefix; wc_attribute_label gives the
			// display name for the taxonomy. Term names come from the
			// variation value (which is a slug for global attributes).
			$taxonomy = preg_replace( '/^attribute_/', '', $key );
			$label    = '';
			if ( taxonomy_exists( $taxonomy ) ) {
				$term = get_term_by( 'slug', $value, $taxonomy );
				if ( $term && ! is_wp_error( $term ) ) {
					$label = $term->name;
				}
			}
			if ( '' === $label ) {
				$label = $value;
			}
			$attribute_values[] = $label;
		}
		$attributes_label = implode( ' / ', $attribute_values );

		if ( '' === $attributes_label ) {
			$attributes_label = __( 'Default', 'wooagent-companion' );
		}

		$variations[] = array(
			'id'               => (int) $variation->get_id(),
			'attributes_label' => $attributes_label,
			'regular_price'    => (string) $variation->get_regular_price(),
			'sale_price'       => (string) $variation->get_sale_price(),
			'stock_status'     => (string) $variation->get_stock_status(),
			'menu_order'       => (int) $variation->get_menu_order(),
		);
	}

	// Sort by menu_order so the UI ordering is stable across requests.
	usort(
		$variations,
		static function ( $a, $b ) {
			return $a['menu_order'] <=> $b['menu_order'];
		}
	);

	return array(
		'parent_id'  => (int) $parent->get_id(),
		'variations' => $variations,
	);
}

function wooagent_products_list_categories_execute( array $args ) {
	$terms = get_terms(
		array(
			'taxonomy'   => 'product_cat',
			'hide_empty' => ! empty( $args['hide_empty'] ),
		)
	);

	if ( is_wp_error( $terms ) ) {
		return $terms;
	}

	$out = array();
	foreach ( $terms as $term ) {
		$out[] = array(
			'id'        => (int) $term->term_id,
			'name'      => (string) $term->name,
			'slug'      => (string) $term->slug,
			'parent_id' => (int) $term->parent,
			'count'     => (int) $term->count,
		);
	}

	return array(
		'categories' => $out,
		'total'      => count( $out ),
	);
}

function wooagent_products_update_execute( array $args ) {
	$product = wc_get_product( (int) $args['id'] );
	if ( ! $product ) {
		return new WP_Error( 'wooagent_product_not_found', __( 'Product not found.', 'wooagent-companion' ), array( 'status' => 404 ) );
	}

	$updated = array();

	if ( array_key_exists( 'name', $args ) ) {
		$product->set_name( $args['name'] );
		$updated[] = 'name';
	}
	if ( array_key_exists( 'description', $args ) ) {
		$product->set_description( $args['description'] );
		$updated[] = 'description';
	}
	if ( array_key_exists( 'short_description', $args ) ) {
		$product->set_short_description( $args['short_description'] );
		$updated[] = 'short_description';
	}
	if ( array_key_exists( 'regular_price', $args ) ) {
		$product->set_regular_price( $args['regular_price'] );
		$updated[] = 'regular_price';
	}
	if ( array_key_exists( 'sale_price', $args ) ) {
		$product->set_sale_price( $args['sale_price'] );
		$updated[] = 'sale_price';
	}
	if ( array_key_exists( 'status', $args ) ) {
		$product->set_status( $args['status'] );
		$updated[] = 'status';
	}

	$product->save();

	if ( array_key_exists( 'meta_title', $args ) ) {
		update_post_meta( $product->get_id(), '_yoast_wpseo_title', $args['meta_title'] );
		$updated[] = 'meta_title';
	}
	if ( array_key_exists( 'meta_description', $args ) ) {
		update_post_meta( $product->get_id(), '_yoast_wpseo_metadesc', $args['meta_description'] );
		$updated[] = 'meta_description';
	}

	return array(
		'id'             => $product->get_id(),
		'updated_fields' => $updated,
		'date_modified'  => $product->get_date_modified() ? $product->get_date_modified()->date( 'c' ) : '',
	);
}

function wooagent_product_to_summary( $product ): array {
	return array(
		'id'                       => $product->get_id(),
		'name'                     => $product->get_name(),
		'slug'                     => $product->get_slug(),
		'sku'                      => (string) $product->get_sku(),
		'status'                   => $product->get_status(),
		'type'                     => $product->get_type(),
		'regular_price'            => (string) $product->get_regular_price(),
		'sale_price'               => (string) $product->get_sale_price(),
		'on_sale'                  => $product->is_on_sale(),
		'description_length'       => strlen( (string) $product->get_description() ),
		'short_description_length' => strlen( (string) $product->get_short_description() ),
		'featured'                 => $product->is_featured(),
		'date_modified'            => $product->get_date_modified() ? $product->get_date_modified()->date( 'c' ) : '',
		'total_sales'              => (int) $product->get_total_sales(),
	);
}
