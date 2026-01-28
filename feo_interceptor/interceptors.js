// ============================================================================
// FEO Interceptor Logic - Standalone JavaScript for Go Goja Runtime
// ============================================================================
// This file contains all the logic for intercepting and merging Chrome service
// responses with local Frontend CRD configurations.
//
// Global functions provided by Go:
// - _GO_READ_FILE(filePath): string
// - _GO_PARSE_YAML(yamlString): object
// - _GO_LOG(message): void
// ============================================================================

// ============================================================================
// Utility Functions
// ============================================================================

/**
 * Converts a string to a valid JavaScript variable name
 * Example: "my-app-123" -> "myApp123"
 */
function jsVarName(s) {
  return (
    s
      // Camel case dashes
      .replace(/-(\w)/g, function(_, match) { return match.toUpperCase(); })
      // Remove leading digits
      .replace(/^[0-9]+/, '')
      // Remove all non alphanumeric chars
      .replace(/[^A-Za-z0-9]+/g, '')
  );
}

/**
 * Type guard to check if a nav item has a segmentRef
 */
function hasSegmentRef(item) {
  return item && 
         typeof item.segmentRef === 'object' && 
         typeof item.segmentRef.segmentId === 'string' && 
         typeof item.segmentRef.frontendName === 'string';
}

// ============================================================================
// URL Matching Functions
// ============================================================================

function matchNavigationRequest(url) {
  return !!url.match(/\/api\/chrome-service\/v1\/static\/bundles-generated\.json/);
}

function matchSearchIndexRequest(url) {
  return !!url.match(/\/api\/chrome-service\/v1\/static\/search-index-generated\.json/);
}

function matchServiceTilesRequest(url) {
  return !!url.match(/\/api\/chrome-service\/v1\/static\/service-tiles-generated\.json/);
}

function matchModulesRequest(url) {
  return !!url.match(/\/api\/chrome-service\/v1\/static\/fed-modules-generated\.json/);
}

function matchWidgetRegistryRequest(url) {
  return !!url.match(/\/api\/chrome-service\/v1\/static\/widget-registry-generated\.json/);
}

// ============================================================================
// Navigation Interceptor Functions
// ============================================================================

/**
 * Get all bundle segments for a specific bundle ID
 */
function getBundleSegments(segmentCache, bundleId) {
  var result = {};
  for (var key in segmentCache) {
    if (segmentCache.hasOwnProperty(key)) {
      var segment = segmentCache[key];
      if (segment.bundleId === bundleId) {
        result[segment.segmentId] = segment;
      }
    }
  }
  return result;
}

/**
 * Find a matching nav item by ID in a nested structure
 */
function findMatchingSegmentItem(navItems, matchId) {
  if (!navItems) return undefined;
  
  for (var i = 0; i < navItems.length; i++) {
    var item = navItems[i];
    if (!hasSegmentRef(item) && item.id === matchId) {
      return item;
    }
  }
  
  for (var i = 0; i < navItems.length; i++) {
    var curr = navItems[i];
    if (!hasSegmentRef(curr)) {
      if (curr.routes) {
        var match = findMatchingSegmentItem(curr.routes, matchId);
        if (match) return match;
      }
      if (curr.navItems) {
        var match = findMatchingSegmentItem(curr.navItems, matchId);
        if (match) return match;
      }
    }
  }
  
  return undefined;
}

/**
 * Handle nested navigation merging
 */
function handleNestedNav(
  segmentMatch,
  originalNavItem,
  bSegmentCache,
  nSegmentCache,
  bundleId,
  currentFrontendName,
  parentSegment
) {
  var routes = segmentMatch.routes;
  var navItems = segmentMatch.navItems;
  var segmentItem = {};
  
  // Copy all properties except routes and navItems
  for (var key in segmentMatch) {
    if (segmentMatch.hasOwnProperty(key) && key !== 'routes' && key !== 'navItems') {
      segmentItem[key] = segmentMatch[key];
    }
  }
  
  var parsedRoutes = originalNavItem.routes;
  var parsedNavItems = originalNavItem.navItems;
  
  // Merge local segment routes with remote routes
  if (routes && routes.length > 0) {
    var remoteRoutes = originalNavItem.routes || [];
    var remoteRoutesMap = {};
    
    for (var i = 0; i < remoteRoutes.length; i++) {
      var route = remoteRoutes[i];
      if (route.id) {
        remoteRoutesMap[route.id] = route;
      }
    }
    
    var mergedRoutes = [];
    for (var i = 0; i < routes.length; i++) {
      var localRoute = routes[i];
      if (localRoute.id && remoteRoutesMap[localRoute.id]) {
        var remoteRoute = remoteRoutesMap[localRoute.id];
        var merged = {};
        for (var key in remoteRoute) {
          if (remoteRoute.hasOwnProperty(key)) {
            merged[key] = remoteRoute[key];
          }
        }
        for (var key in localRoute) {
          if (localRoute.hasOwnProperty(key)) {
            merged[key] = localRoute[key];
          }
        }
        mergedRoutes.push(merged);
      } else {
        mergedRoutes.push(localRoute);
      }
    }
    
    // Add remote routes that don't exist in local routes
    for (var i = 0; i < remoteRoutes.length; i++) {
      var remoteRoute = remoteRoutes[i];
      var found = false;
      if (remoteRoute.id) {
        for (var j = 0; j < routes.length; j++) {
          if (routes[j].id === remoteRoute.id) {
            found = true;
            break;
          }
        }
      }
      if (!found) {
        mergedRoutes.push(remoteRoute);
      }
    }
    
    parsedRoutes = parseNavItems(mergedRoutes, bSegmentCache, nSegmentCache, bundleId, currentFrontendName);
  } else if (parsedRoutes) {
    parsedRoutes = parseNavItems(parsedRoutes, bSegmentCache, nSegmentCache, bundleId, currentFrontendName);
  }
  
  // Merge local segment navItems with remote navItems (similar logic)
  if (navItems && navItems.length > 0) {
    var remoteNavItems = originalNavItem.navItems || [];
    var remoteNavItemsMap = {};
    
    for (var i = 0; i < remoteNavItems.length; i++) {
      var navItem = remoteNavItems[i];
      if (navItem.id) {
        remoteNavItemsMap[navItem.id] = navItem;
      }
    }
    
    var mergedNavItems = [];
    for (var i = 0; i < navItems.length; i++) {
      var localNavItem = navItems[i];
      if (localNavItem.id && remoteNavItemsMap[localNavItem.id]) {
        var remoteNavItem = remoteNavItemsMap[localNavItem.id];
        var merged = {};
        for (var key in remoteNavItem) {
          if (remoteNavItem.hasOwnProperty(key)) {
            merged[key] = remoteNavItem[key];
          }
        }
        for (var key in localNavItem) {
          if (localNavItem.hasOwnProperty(key)) {
            merged[key] = localNavItem[key];
          }
        }
        mergedNavItems.push(merged);
      } else {
        mergedNavItems.push(localNavItem);
      }
    }
    
    // Add remote navItems that don't exist in local navItems
    for (var i = 0; i < remoteNavItems.length; i++) {
      var remoteNavItem = remoteNavItems[i];
      var found = false;
      if (remoteNavItem.id) {
        for (var j = 0; j < navItems.length; j++) {
          if (navItems[j].id === remoteNavItem.id) {
            found = true;
            break;
          }
        }
      }
      if (!found) {
        mergedNavItems.push(remoteNavItem);
      }
    }
    
    parsedNavItems = parseNavItems(mergedNavItems, bSegmentCache, nSegmentCache, bundleId, currentFrontendName);
  } else if (parsedNavItems) {
    parsedNavItems = parseNavItems(parsedNavItems, bSegmentCache, nSegmentCache, bundleId, currentFrontendName);
  }
  
  var result = {};
  for (var key in originalNavItem) {
    if (originalNavItem.hasOwnProperty(key)) {
      result[key] = originalNavItem[key];
    }
  }
  for (var key in segmentItem) {
    if (segmentItem.hasOwnProperty(key)) {
      result[key] = segmentItem[key];
    }
  }
  result.position = parentSegment.position;
  result.routes = parsedRoutes;
  result.navItems = parsedNavItems;
  
  return result;
}

/**
 * Find the first index of a segment item for a specific frontend
 */
function findNavItemsFirstSegmentIndex(navItems, frontendName) {
  for (var i = 0; i < navItems.length; i++) {
    var item = navItems[i];
    if (hasSegmentRef(item) && item.segmentRef.frontendName === frontendName) {
      return i;
    }
  }
  return -1;
}

/**
 * Find the length of a segment sequence
 */
function findSegmentSequenceLength(navItems, sequenceStartIndex, segmentId, frontendName) {
  var finalIndex = sequenceStartIndex;
  for (var i = sequenceStartIndex; i < navItems.length; i++) {
    var item = navItems[i];
    var prev = navItems[i - 1];
    
    if (!prev) {
      finalIndex = i;
      continue;
    }
    
    if (item.segmentRef && 
        item.segmentRef.segmentId === segmentId && 
        item.segmentRef.frontendName === frontendName) {
      finalIndex = i;
    } else {
      break;
    }
  }
  return finalIndex - sequenceStartIndex + 1;
}

/**
 * Parse and merge navigation items recursively
 */
function parseNavItems(navItems, bSegmentCache, nSegmentCache, bundleId, currentFrontendName) {
  var relevantSegments = getBundleSegments(bSegmentCache, bundleId);
  var res = [];
  
  for (var i = 0; i < navItems.length; i++) {
    var navItem = navItems[i];
    if (!hasSegmentRef(navItem) && navItem.id) {
      var id = navItem.id;
      var bundleSegmentRef = navItem.bundleSegmentRef;
      
      if (navItem.frontendRef === currentFrontendName && 
          bundleSegmentRef && 
          relevantSegments[bundleSegmentRef]) {
        var parentSegment = relevantSegments[bundleSegmentRef];
        var segmentItemMatch = findMatchingSegmentItem(parentSegment.navItems, id);
        
        if (segmentItemMatch && !hasSegmentRef(segmentItemMatch)) {
          res.push(handleNestedNav(
            segmentItemMatch, 
            navItem, 
            bSegmentCache, 
            nSegmentCache, 
            bundleId, 
            currentFrontendName, 
            parentSegment
          ));
          continue;
        }
      } else if (typeof navItem.groupId === 'string' && navItem.groupId.length > 0) {
        var parsed = {};
        for (var key in navItem) {
          if (navItem.hasOwnProperty(key)) {
            parsed[key] = navItem[key];
          }
        }
        parsed.navItems = parseNavItems(navItem.navItems || [], bSegmentCache, nSegmentCache, bundleId, currentFrontendName);
        res.push(parsed);
        continue;
      }
    }
    res.push(navItem);
  }
  
  // Replace segment sequence with the segment data
  var segmentIndex = findNavItemsFirstSegmentIndex(res, currentFrontendName);
  var iterations = 0;
  
  while (segmentIndex > -1 && iterations < 100) {
    var segment = res[segmentIndex];
    if (hasSegmentRef(segment)) {
      var replacement = nSegmentCache[segment.segmentRef.segmentId];
      if (replacement && replacement.navItems) {
        var replaceLength = findSegmentSequenceLength(res, segmentIndex, segment.segmentRef.segmentId, currentFrontendName);
        var nestedNavItems = [];
        
        for (var i = 0; i < replacement.navItems.length; i++) {
          var navItem = replacement.navItems[i];
          if (navItem.routes) {
            var parsed = {};
            for (var key in navItem) {
              if (navItem.hasOwnProperty(key)) {
                parsed[key] = navItem[key];
              }
            }
            parsed.routes = parseNavItems(navItem.routes, bSegmentCache, nSegmentCache, bundleId, currentFrontendName);
            nestedNavItems.push(parsed);
          } else if (navItem.navItems) {
            var parsed = {};
            for (var key in navItem) {
              if (navItem.hasOwnProperty(key)) {
                parsed[key] = navItem[key];
              }
            }
            parsed.navItems = parseNavItems(navItem.navItems, bSegmentCache, nSegmentCache, bundleId, currentFrontendName);
            nestedNavItems.push(parsed);
          } else {
            nestedNavItems.push(navItem);
          }
        }
        
        // Splice the segment sequence out and insert the nested nav items
        var before = res.slice(0, segmentIndex);
        var after = res.slice(segmentIndex + replaceLength);
        res = before.concat(nestedNavItems).concat(after);
      }
    }
    
    segmentIndex = findNavItemsFirstSegmentIndex(res, currentFrontendName);
    iterations++;
  }
  
  return res;
}

/**
 * Navigation Interceptor - Main function to merge local nav with remote nav
 */
function navigationInterceptor(frontendCRD, nav, bundleName) {
  var res = [];
  var bundleSegmentsCache = {};
  var navSegmentCache = {};
  
  for (var objIdx = 0; objIdx < frontendCRD.objects.length; objIdx++) {
    var obj = frontendCRD.objects[objIdx];
    var bundleSegments = obj.spec.bundleSegments || [];
    
    for (var i = 0; i < bundleSegments.length; i++) {
      var bundleSegment = bundleSegments[i];
      bundleSegmentsCache[bundleSegment.segmentId] = bundleSegment;
    }
    
    var navSegments = obj.spec.navigationSegments || [];
    for (var i = 0; i < navSegments.length; i++) {
      var navSegment = navSegments[i];
      if (navSegment.segmentId) {
        navSegmentCache[navSegment.segmentId] = navSegment;
      }
    }
    
    var missingSegments = [];
    for (var i = 0; i < bundleSegments.length; i++) {
      var segment = bundleSegments[i];
      if (segment.bundleId !== bundleName) {
        continue;
      }
      
      var found = false;
      for (var j = 0; j < nav.navItems.length; j++) {
        if (nav.navItems[j].bundleSegmentRef === segment.segmentId) {
          found = true;
          break;
        }
      }
      
      if (!found) {
        missingSegments.push(segment);
      }
    }
    
    var missingNavItems = [];
    for (var i = 0; i < missingSegments.length; i++) {
      var segment = missingSegments[i];
      for (var j = 0; j < segment.navItems.length; j++) {
        var navItem = {};
        for (var key in segment.navItems[j]) {
          if (segment.navItems[j].hasOwnProperty(key)) {
            navItem[key] = segment.navItems[j][key];
          }
        }
        navItem.position = segment.position;
        missingNavItems.push(navItem);
      }
    }
    
    var parseInput = nav.navItems.concat(missingNavItems);
    res = parseNavItems(parseInput, bundleSegmentsCache, navSegmentCache, bundleName, obj.metadata.name);
  }
  
  // Sort top level segments based on position
  res.sort(function(a, b) {
    if (typeof a.position !== 'number' || typeof b.position !== 'number') {
      return 0;
    }
    return a.position - b.position;
  });
  
  return res;
}

// ============================================================================
// Search Index Interceptor
// ============================================================================

function searchInterceptor(staticSearchIndex, frontendCRD) {
  var frontendRef = frontendCRD.objects[0].metadata.name;
  var result = [];
  
  for (var i = 0; i < staticSearchIndex.length; i++) {
    var entry = staticSearchIndex[i];
    if (entry.frontendRef !== frontendRef) {
      result.push(entry);
    }
  }
  
  var newEntries = frontendCRD.objects[0].spec.searchEntries || [];
  return result.concat(newEntries);
}

// ============================================================================
// Module Interceptor
// ============================================================================

function moduleInterceptor(moduleRegistry, frontendCRD) {
  var moduleName = jsVarName(frontendCRD.objects[0].metadata.name);
  var cdnPath = frontendCRD.objects[0].spec.frontend.paths[0];
  var result = {};
  
  for (var key in moduleRegistry) {
    if (moduleRegistry.hasOwnProperty(key)) {
      result[key] = moduleRegistry[key];
    }
  }
  
  var moduleSpec = {};
  for (var key in frontendCRD.objects[0].spec.module) {
    if (frontendCRD.objects[0].spec.module.hasOwnProperty(key)) {
      moduleSpec[key] = frontendCRD.objects[0].spec.module[key];
    }
  }
  moduleSpec.cdnPath = cdnPath;
  
  result[moduleName] = moduleSpec;
  return result;
}

// ============================================================================
// Service Tiles Interceptor
// ============================================================================

function serviceTilesInterceptor(serviceCategories, frontendCrd) {
  var frontendRef = frontendCrd.objects[0].metadata.name;
  var result = [];
  
  for (var i = 0; i < serviceCategories.length; i++) {
    result.push(serviceCategories[i]);
  }
  
  var frontendCategories = {};
  var serviceTiles = frontendCrd.objects[0].spec.serviceTiles || [];
  
  for (var i = 0; i < serviceTiles.length; i++) {
    var tile = serviceTiles[i];
    var section = tile.section;
    var group = tile.group;
    
    if (!frontendCategories[section]) {
      frontendCategories[section] = {};
    }
    
    if (!frontendCategories[section][group]) {
      frontendCategories[section][group] = [];
    }
    
    frontendCategories[section][group].push(tile);
  }
  
  result = result.map(function(category) {
    var newGroups = category.links.map(function(group) {
      var newTiles = [];
      
      for (var i = 0; i < group.links.length; i++) {
        var tile = group.links[i];
        if (tile.frontendRef !== frontendRef) {
          newTiles.push(tile);
        }
      }
      
      var additionalTiles = (frontendCategories[category.id] && frontendCategories[category.id][group.id]) || [];
      
      return {
        id: group.id,
        isGroup: group.isGroup,
        title: group.title,
        links: newTiles.concat(additionalTiles)
      };
    });
    
    return {
      description: category.description,
      icon: category.icon,
      id: category.id,
      links: newGroups
    };
  });
  
  return result;
}

// ============================================================================
// Widget Registry Interceptor
// ============================================================================

function widgetRegistryInterceptor(widgetEntries, frontendCrd) {
  var frontendName = frontendCrd.objects[0].metadata.name;
  var result = [];
  
  for (var i = 0; i < widgetEntries.length; i++) {
    var entry = widgetEntries[i];
    if (entry.frontendRef !== frontendName) {
      result.push(entry);
    }
  }
  
  var newEntries = frontendCrd.objects[0].spec.widgetRegistry || [];
  return result.concat(newEntries);
}

// ============================================================================
// Main Entry Point
// ============================================================================

/**
 * Process a request by intercepting and merging remote data with local CRD config
 * 
 * @param {string} requestUrl - The URL of the request being intercepted
 * @param {string} remoteBodyRaw - The raw JSON response from the remote server
 * @param {string} crdPath - The file path to the local Frontend CRD YAML file
 * @returns {string} The merged JSON response as a string
 */
function processRequest(requestUrl, remoteBodyRaw, crdPath) {
  try {
    // Read and parse the local CRD file
    var crdFileContent = _GO_READ_FILE(crdPath);
    var frontendCRD = _GO_PARSE_YAML(crdFileContent);
    
    // Check if FEO features are enabled
    if (!frontendCRD.objects || 
        !frontendCRD.objects[0] || 
        !frontendCRD.objects[0].spec.feoConfigEnabled) {
      _GO_LOG('FEO features not enabled, returning original response');
      return remoteBodyRaw;
    }
    
    // Parse the remote response
    var remoteBody;
    try {
      remoteBody = JSON.parse(remoteBodyRaw);
    } catch (e) {
      _GO_LOG('Error parsing remote response JSON: ' + e.toString());
      return remoteBodyRaw;
    }
    
    var result;
    
    // Route to the appropriate interceptor based on the URL
    if (matchNavigationRequest(requestUrl)) {
      _GO_LOG('Processing navigation request');
      var resultBundles = [];
      for (var i = 0; i < remoteBody.length; i++) {
        var bundle = remoteBody[i];
        var navItems = navigationInterceptor(frontendCRD, bundle, bundle.id);
        resultBundles.push({
          id: bundle.id,
          title: bundle.title,
          navItems: navItems
        });
      }
      result = JSON.stringify(resultBundles);
    } else if (matchSearchIndexRequest(requestUrl)) {
      _GO_LOG('Processing search index request');
      var searchResult = searchInterceptor(remoteBody, frontendCRD);
      result = JSON.stringify(searchResult);
    } else if (matchServiceTilesRequest(requestUrl)) {
      _GO_LOG('Processing service tiles request');
      var tilesResult = serviceTilesInterceptor(remoteBody, frontendCRD);
      result = JSON.stringify(tilesResult);
    } else if (matchModulesRequest(requestUrl)) {
      _GO_LOG('Processing modules request');
      var modulesResult = moduleInterceptor(remoteBody, frontendCRD);
      result = JSON.stringify(modulesResult);
    } else if (matchWidgetRegistryRequest(requestUrl)) {
      _GO_LOG('Processing widget registry request');
      var widgetResult = widgetRegistryInterceptor(remoteBody, frontendCRD);
      result = JSON.stringify(widgetResult);
    } else {
      _GO_LOG('No matching interceptor for URL, returning original response');
      result = remoteBodyRaw;
    }
    
    return result;
  } catch (error) {
    _GO_LOG('Error in processRequest: ' + error.toString());
    return remoteBodyRaw;
  }
}